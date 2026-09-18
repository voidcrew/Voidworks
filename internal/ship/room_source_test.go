package ship

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceFlagReadsOnlyUnambiguousOwnLiterals(t *testing.T) {
	for _, tc := range []struct {
		body                     string
		value, explicit, invalid bool
	}{
		{"\tplayer_hidden = TRUE // hidden\n", true, true, false},
		{"\tplayer_hidden = 1\r\n", true, true, false},
		{"\tplayer_hidden = FALSE\n", false, true, false},
		{"\tplayer_hidden = 0\n", false, true, false},
		{"\tjobs = list(\n\t\tplayer_hidden = TRUE,\n\t)\n", false, false, false},
		{"\tdesc = \"player_hidden = TRUE\"\n\t// player_hidden = TRUE\n", false, false, false},
		{"\tplayer_hidden = CUSTOM_FLAG\n", false, true, true},
		{"\tplayer_hidden = TRUE\n\tplayer_hidden = FALSE\n", false, false, true},
	} {
		source := []byte("/datum/ship\n" + tc.body + "/datum/ship/other\n\tplayer_hidden = TRUE\n")
		value, explicit, err := sourceFlag(source, "/datum/ship", "player_hidden")
		if value != tc.value || explicit != tc.explicit || (err != nil) != tc.invalid {
			t.Fatalf("%q: %v, %v, %v", tc.body, value, explicit, err)
		}
	}
}

func TestRewriteRoomSlotsPreservesOtherDefinitions(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		prefix := "/* /datum/ship_theme/sample\n\tupgrade_slot_ids = list(\"ignore\") */\n/datum/ship_theme/sample\n\tdesc = \"list(fake) // text\"\n\tupgrade_slot_ids = "
		expression := "list(\n\t\t\"old\", // old room\n\t)"
		suffix := " // Keep this comment\n\tjob_slots = list(list(name = \"Engineer\", slots = 2))\n\tpart_cost = list(PART_CLASS_TRADE = 17)\n\n/datum/ship_theme/sample/other\n\tupgrade_slot_ids = list(\"old\")\n"
		original := strings.ReplaceAll(prefix+expression+suffix, "\n", newline)
		got, err := rewriteRoomSlots([]byte(original), "/datum/ship_theme/sample", []string{"old"}, []string{"old", "new_room"})
		if err != nil {
			t.Fatal(err)
		}
		want := strings.ReplaceAll(prefix+`list("old", "new_room")`+suffix, "\n", newline)
		if string(got) != want {
			t.Fatalf("unexpected source change: %s", got)
		}
		if _, err = rewriteRoomSlots([]byte(original), "/datum/ship_theme/sample", []string{"stale"}, []string{"stale", "new_room"}); err == nil {
			t.Fatal("accepted stale environment")
		}
	}
}

func TestRewriteInheritedAndUnsupportedRoomLists(t *testing.T) {
	for _, source := range []string{"/datum/ship_theme/sample", "/datum/ship_theme/sample\n\tname = \"Sample\"\n"} {
		got, err := rewriteRoomSlots([]byte(source), "/datum/ship_theme/sample", nil, []string{"base", "new"})
		if err != nil || !bytes.Contains(got, []byte("\n\tupgrade_slot_ids = list(\"base\", \"new\")\n")) {
			t.Fatalf("could not add inherited list: %v %s", err, got)
		}
	}
	for _, value := range []string{"DEFAULT_SLOTS", "list(\"old\") + list(\"other\")", "nullified", "list(\"old\""} {
		_, err := rewriteRoomSlots([]byte("/datum/ship_theme/sample\n\tupgrade_slot_ids = "+value+"\n"), "/datum/ship_theme/sample", []string{"old"}, []string{"old", "new"})
		if err == nil {
			t.Fatal("accepted unsupported room list:", value)
		}
	}
}

func TestRewriteModularFlag(t *testing.T) {
	const typePath = "/datum/map_template/shuttle/voidcrew/sample"
	for _, newline := range []string{"\n", "\r\n"} {
		inherited := strings.ReplaceAll("// has_upgrade_slots = FALSE in a comment\n"+typePath+"\n\tname = \"Sample\"\n\tsuffix = \"sample\"\n\n"+typePath+"/other\n\thas_upgrade_slots = FALSE\n", "\n", newline)
		got, err := rewriteModularFlag([]byte(inherited), typePath)
		want := strings.ReplaceAll("// has_upgrade_slots = FALSE in a comment\n"+typePath+"\n\thas_upgrade_slots = TRUE\n\tname = \"Sample\"\n\tsuffix = \"sample\"\n\n"+typePath+"/other\n\thas_upgrade_slots = FALSE\n", "\n", newline)
		if err != nil || string(got) != want {
			t.Fatalf("inherited flag: %v %s", err, got)
		}
		disabled := strings.ReplaceAll(typePath+"\n\tname = \"Sample\"\n\thas_upgrade_slots = FALSE // fixed layout\n", "\n", newline)
		got, err = rewriteModularFlag([]byte(disabled), typePath)
		want = strings.ReplaceAll(typePath+"\n\tname = \"Sample\"\n\thas_upgrade_slots = TRUE // fixed layout\n", "\n", newline)
		if err != nil || string(got) != want {
			t.Fatalf("disabled flag: %v %s", err, got)
		}
		enabled := strings.ReplaceAll(typePath+"\n\thas_upgrade_slots = TRUE\n", "\n", newline)
		if got, err = rewriteModularFlag([]byte(enabled), typePath); err != nil || string(got) != enabled {
			t.Fatalf("enabled flag changed: %v %s", err, got)
		}
	}
	// The slot list and the flag are inserted together at the top of a block.
	both, err := rewriteRoomSlots([]byte(typePath+"\n\tname = \"Sample\"\n"), typePath, nil, []string{"bay"})
	if err != nil {
		t.Fatal(err)
	}
	if both, err = rewriteModularFlag(both, typePath); err != nil || string(both) != typePath+"\n\thas_upgrade_slots = TRUE\n\tupgrade_slot_ids = list(\"bay\")\n\tname = \"Sample\"\n" {
		t.Fatalf("combined insertion: %v %s", err, both)
	}
	for _, value := range []string{"SHIP_SLOTS", "!FALSE"} {
		if _, err := rewriteModularFlag([]byte(typePath+"\n\thas_upgrade_slots = "+value+"\n"), typePath); err == nil {
			t.Fatal("accepted unsupported flag value:", value)
		}
	}
	if _, err := rewriteModularFlag([]byte(typePath+"\n\thas_upgrade_slots = FALSE\n"+typePath+"\n"), typePath); err == nil {
		t.Fatal("accepted a duplicated definition block")
	}
}
