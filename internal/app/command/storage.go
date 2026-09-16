package command

import "github.com/rs/zerolog/log"

// NullSpaceStackId is for a stack which won't hold any command and will be always empty.
const NullSpaceStackId = "__NULL_SPACE__"

// Storage is used to store application command and handle undo/redo stuff.
type Storage struct {
	currentStackId string
	commandStacks  map[string]*commandStack
}

func NewStorage() *Storage {
	s := &Storage{commandStacks: make(map[string]*commandStack)}
	s.SetStack(NullSpaceStackId)
	return s
}

func (s *Storage) Free() {
	s.commandStacks = make(map[string]*commandStack, len(s.commandStacks))
	s.SetStack(NullSpaceStackId)
	log.Print("storage free")
}

func (s *Storage) SetStack(id string) {
	if s.currentStackId == id {
		return
	}

	log.Print("changing stack to:", id)

	s.currentStackId = id
	if _, ok := s.commandStacks[id]; !ok {
		s.commandStacks[id] = &commandStack{id: id}
		log.Print("created stack:", id)
	}
}

func (s *Storage) DisposeStack(id string) {
	if id == NullSpaceStackId {
		log.Print("skip disposing for:", id)
		return
	}

	log.Print("disposing stack:", id)
	delete(s.commandStacks, id)
	if s.currentStackId == id {
		s.SetStack(NullSpaceStackId)
	}
}

func (s *Storage) Push(command Command) {
	s.PushV(s.currentStackId, command)
}

// PushV records a command for its source workspace, even after focus changes.
func (s *Storage) PushV(id string, command Command) {
	if id == NullSpaceStackId {
		log.Print("skip pushing for:", id)
		return
	}

	if stack, ok := s.commandStacks[id]; ok {
		logStackAction(stack, "push command: "+command.name)
		stack.undo = append(stack.undo, command)
		clear(stack.redo)
		stack.redo = stack.redo[:0]
		stack.balance++
	} else {
		logNoStackAvailable("push command")
	}
}

// LimitHistory bounds memory for pixel documents, while retaining at least the
// latest action. Map/workshop histories keep their existing behavior.
func (s *Storage) LimitHistory(id string, count int, bytes int64) {
	stack := s.commandStacks[id]
	if stack == nil || len(stack.undo) < 2 {
		return
	}
	start := len(stack.undo) - 1
	cost := stack.undo[start].cost
	for start > 0 && len(stack.undo)-start < count && cost+stack.undo[start-1].cost <= bytes {
		start--
		cost += stack.undo[start].cost
	}
	if start > 0 {
		stack.undo = append([]Command(nil), stack.undo[start:]...)
		stack.balance = min(stack.balance, len(stack.undo))
	}
}

func (s *Storage) Undo() {
	s.UndoV(s.currentStackId)
}

func (s *Storage) UndoV(id string) {
	if stack, ok := s.commandStacks[id]; ok {
		logStackAction(stack, "undo")

		if len(stack.undo) == 0 {
			log.Print("unable to undo empty stack")
			return
		}

		s.undo(stack)
	} else {
		logNoStackAvailable("undo")
	}
}

func (s *Storage) undo(stack *commandStack) {
	var command Command
	command, stack.undo = stack.undo[len(stack.undo)-1], stack.undo[:len(stack.undo)-1]
	stack.redo = append(stack.redo, command.Run())
	stack.balance--
}

func (s *Storage) Redo() {
	s.RedoV(s.currentStackId)
}

func (s *Storage) RedoV(id string) {
	if stack, ok := s.commandStacks[id]; ok {
		logStackAction(stack, "redo")

		if len(stack.redo) == 0 {
			log.Print("unable to read empty stack")
			return
		}

		s.redo(stack)
	} else {
		logNoStackAvailable("redo")
	}
}

func (s *Storage) redo(stack *commandStack) {
	var command Command
	command, stack.redo = stack.redo[len(stack.redo)-1], stack.redo[:len(stack.redo)-1]
	stack.undo = append(stack.undo, command.Run())
	stack.balance++
}

func (s *Storage) HasUndo() bool {
	return s.HasUndoV(s.currentStackId)
}

// UndoName is the label of the command the next Undo would run.
func (s *Storage) UndoName() string {
	if stack, ok := s.commandStacks[s.currentStackId]; ok && len(stack.undo) > 0 {
		return stack.undo[len(stack.undo)-1].ReadableName()
	}
	return ""
}

func (s *Storage) HasUndoV(id string) bool {
	if stack, ok := s.commandStacks[id]; ok {
		return len(stack.undo) > 0
	}
	return false
}

func (s *Storage) HasRedo() bool {
	return s.HasRedoV(s.currentStackId)
}

func (s *Storage) HasRedoV(id string) bool {
	if stack, ok := s.commandStacks[id]; ok {
		return len(stack.redo) > 0
	}
	return false
}

func (s *Storage) IsModified(id string) bool {
	if stack, ok := s.commandStacks[id]; ok {
		return stack.balance != 0 || stack.appliedCommandId() != stack.balanceCommandId
	}
	return false
}

func (s *Storage) ForceBalance(id string) {
	if id == NullSpaceStackId {
		log.Print("skipping force balance for:", id)
		return
	}

	if stack, ok := s.commandStacks[id]; ok {
		logStackAction(stack, "force balance")
		stack.balance = 0
		stack.balanceCommandId = stack.appliedCommandId()
	}
}

func (s *Storage) Balance(id string) {
	if id == NullSpaceStackId {
		log.Print("skipping balance for:", id)
		return
	}

	if stack, ok := s.commandStacks[id]; ok {
		logStackAction(stack, "balance")

		for {
			if stack.balance == 0 {
				break
			} else if stack.balance > 0 {
				s.undo(stack)
			} else if stack.balance < 0 {
				s.redo(stack)
			}
		}
	}
}

func logNoStackAvailable(action string) {
	log.Print("invalid action, no stack available at the moment:", action)
}

func logStackAction(stack *commandStack, action string) {
	log.Printf("stack action [%s] on [%s]", action, stack.id)
}

type commandStack struct {
	id      string
	balance int
	undo    []Command
	redo    []Command

	// Field stores a command id at the moment when the stack was forcefully balanced.
	balanceCommandId uint64
}

func (c commandStack) appliedCommandId() uint64 {
	if len(c.undo) > 0 {
		return c.undo[len(c.undo)-1].id
	}
	return 0
}
