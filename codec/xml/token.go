package xml

type Attr struct {
	Name  string
	Space string
	Value string
}

type StartElement struct {
	Name  string
	Space string
	Attrs []Attr
}

type EndElement struct {
	Name  string
	Space string
}

type CharData struct {
	Text string
}

type Comment struct {
	Text string
}

type ProcInst struct {
	Target string
	Inst   string
}

type Directive struct {
	Text string
}
