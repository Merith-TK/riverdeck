package wsdevice

// InputDescriptor holds the declared capabilities of one client input.
type InputDescriptor struct {
	ID          string
	Type        string // "button" or "dial"
	X, Y        int
	HasXY       bool
	Image       bool
	ImageWidth  int
	ImageHeight int
	Text        bool
	Formats     []string // per-input format override; nil = use device default
}

// wsInbound is the union of all client -> server message types.
type wsInbound struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Event string `json:"event"`
}

// helloMsg is the first message a client sends after connecting.
type helloMsg struct {
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Rows    int         `json:"rows"`
	Cols    int         `json:"cols"`
	Formats []string    `json:"formats"`
	Inputs  []inputSpec `json:"inputs"`
}

// inputSpec describes a single input within a hello message.
type inputSpec struct {
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	X       *int        `json:"x"`
	Y       *int        `json:"y"`
	Display displaySpec `json:"display"`
}

// displaySpec describes the display capabilities of an input.
type displaySpec struct {
	Image       bool     `json:"image"`
	ImageWidth  int      `json:"imageWidth"`
	ImageHeight int      `json:"imageHeight"`
	Text        bool     `json:"text"`
	Formats     []string `json:"formats"`
}
