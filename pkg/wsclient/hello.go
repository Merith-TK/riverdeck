package wsclient

import "fmt"

type HelloInput struct {
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	X       int          `json:"x"`
	Y       int          `json:"y"`
	Display HelloDisplay `json:"display"`
}

type HelloDisplay struct {
	Image       bool     `json:"image"`
	ImageWidth  int      `json:"imageWidth"`
	ImageHeight int      `json:"imageHeight"`
	Text        bool     `json:"text"`
	Formats     []string `json:"formats"`
}

type HelloMsg struct {
	Type    string       `json:"type"`
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Rows    int          `json:"rows"`
	Cols    int          `json:"cols"`
	Formats []string     `json:"formats"`
	Inputs  []HelloInput `json:"inputs"`
}

func BuildHelloMsg(deviceID, name string, rows, cols, pixelSize int, formats []string) HelloMsg {
	inputs := make([]HelloInput, rows*cols)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			inputs[idx] = HelloInput{
				ID:   fmt.Sprintf("btn%d", idx),
				Type: "button",
				X:    col,
				Y:    row,
				Display: HelloDisplay{
					Image:       true,
					ImageWidth:  pixelSize,
					ImageHeight: pixelSize,
					Text:        true,
					Formats:     formats,
				},
			}
		}
	}
	return HelloMsg{
		Type:    "hello",
		ID:      deviceID,
		Name:    name,
		Rows:    rows,
		Cols:    cols,
		Formats: formats,
		Inputs:  inputs,
	}
}
