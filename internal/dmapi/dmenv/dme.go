package dmenv

import (
	"path/filepath"
	"sdmm/third_party/sdmmparser"
	"strings"

	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmvars"
)

type Dme struct {
	Name     string
	RootDir  string
	RootFile string
	Objects  map[string]*Object
	preview  *previewParse
}

// previewParse is one run of the compiled-appearance parser. env and err are
// written before done closes and may only be read after it.
type previewParse struct {
	done chan struct{}
	env  *Dme
	err  error
}

func New(path string) (*Dme, error) {
	return newEnvironment(path, sdmmparser.ParseEnvironment)
}

// The preview field belongs to the UI thread; only the parse itself runs apart.
func (d *Dme) startPreviewParse() *previewParse {
	if d.preview == nil {
		parse := &previewParse{done: make(chan struct{})}
		d.preview = parse
		go func(path string) {
			defer close(parse.done)
			parse.env, parse.err = newEnvironment(path, sdmmparser.ParsePreviewEnvironment)
		}(d.RootFile)
	}
	return d.preview
}

// PreviewEnvironment is cached for this loaded project and never replaces the
// editor's definitions. Reopening the project invalidates the cache naturally.
// It waits for the parse; a parse that failed runs again on the next call.
func (d *Dme) PreviewEnvironment() (*Dme, error) {
	parse := d.startPreviewParse()
	<-parse.done
	if parse.err != nil {
		d.preview = nil
	}
	return parse.env, parse.err
}

// WarmPreviewEnvironment starts the parse ahead of the first preview.
func (d *Dme) WarmPreviewEnvironment() { d.startPreviewParse() }

// PollPreviewEnvironment is PreviewEnvironment for callers that must not wait.
// Until the parse finishes it returns ready as false.
func (d *Dme) PollPreviewEnvironment() (env *Dme, ready bool, err error) {
	parse := d.startPreviewParse()
	select {
	case <-parse.done:
		return parse.env, true, parse.err
	default:
		return nil, false, nil
	}
}

func newEnvironment(path string, parse func(string) (*sdmmparser.ObjectTreeType, error)) (*Dme, error) {
	dme := Dme{
		Name:     filepath.Base(path),
		RootDir:  filepath.Dir(path),
		RootFile: path,
		Objects:  make(map[string]*Object),
	}

	objectTreeType, err := parse(path)
	if err != nil {
		return nil, err
	}

	traverseTree0(objectTreeType, "", nil, &dme)

	for _, object := range dme.Objects {
		if parentType, ok := object.Vars.Value("parent_type"); ok {
			object.parent = dme.Objects[parentType]
		}
		if object.parent != nil {
			object.Vars.LinkParent(object.parent.Vars)
		}
	}

	return &dme, nil
}

func nameFromPath(path string, parentName string) string {
	if parentName == "" && len(path) > 1 {
		return "\"" + dm.PathLast(path) + "\""
	}
	return parentName
}

func traverseTree0(root *sdmmparser.ObjectTreeType, parentName string, parent *Object, dme *Dme) {
	variables := dmvars.MutableVariables{}
	varFlags := make(map[string]VarFlags, len(root.Vars))
	var name string

	for _, treeVar := range root.Vars {
		value := sanitizeVar(treeVar.Value)

		if treeVar.Name == "name" {
			if value == dmvars.NullValue {
				value = nameFromPath(root.Path, parentName)
			}

			name = value
		}

		variables.Put(treeVar.Name, value)

		if treeVar.Decl {
			var flags VarFlags
			flags.Tmp = treeVar.IsTmp
			flags.Const = treeVar.IsConst
			flags.Static = treeVar.IsStatic
			varFlags[treeVar.Name] = flags
		}
	}

	if _, ok := variables.Value("name"); !ok {
		variables.Put("name", nameFromPath(root.Path, parentName))
	}

	object := &Object{
		env:      dme,
		parent:   parent,
		Path:     root.Path,
		Vars:     variables.ToImmutable(),
		VarFlags: varFlags,
		Location: root.Location,
	}

	children := make([]string, 0, len(root.Children))
	for _, child := range root.Children {
		children = append(children, child.Path)
		traverseTree0(&child, name, object, dme)
	}

	object.DirectChildren = children
	dme.Objects[root.Path] = object
}

func sanitizeVar(value string) string {
	if len(value) > 2 && strings.HasPrefix(value, "{\"") && strings.HasSuffix(value, "\"}") {
		value = value[1 : len(value)-1]
		return value
	}
	return value
}
