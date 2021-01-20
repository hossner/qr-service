package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"

	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/experimental/devices/inky"
	"periph.io/x/periph/host"
)

const (
	defaultPort = ":7760"
	defaultHost = ""
)

type cmdStruct struct {
	Cmd  string `json:"cmd"`
	Data []byte `json:"data"`
}

var dc gpio.PinIO
var reset gpio.PinIO
var busy gpio.PinIO
var portCloser spi.PortCloser
var dev *inky.Dev

func main() {
	if _, err := host.Init(); err != nil {
		fmt.Println("Err init host:", err.Error())
		return
	}
	err := error(nil)
	portCloser, err = spireg.Open("SPI0.0")
	if err != nil {
		fmt.Println("Err opening SPI device:", err.Error())
		return
	}

	dc = gpioreg.ByName("22")
	reset = gpioreg.ByName("27")
	busy = gpioreg.ByName("17")

	dev, err = inky.New(portCloser, dc, reset, busy, &inky.Opts{
		Model:       inky.WHAT,
		ModelColor:  inky.Red,
		BorderColor: inky.Black,
	})
	if err != nil {
		fmt.Println("Err connecting to inky display:", err.Error())
		return
	}

	http.HandleFunc("/", handler)
	http.ListenAndServe(defaultHost+defaultPort, nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("In handler...")

	d := json.NewDecoder(r.Body)
	cmd := &cmdStruct{}
	if d.Decode(cmd) != nil {
		fmt.Fprintf(w, "{\"resp\": \"err\"}")
		fmt.Println("Err json-decoding body")
		return
	}

	img, err := png.Decode(bytes.NewReader(cmd.Data))
	if err != nil {
		fmt.Fprintf(w, "{\"resp\": \"err\"}")
		fmt.Println("Err decoding img:", err.Error())
		return
	}

	if err := dev.Draw(img.Bounds(), img, image.Point{}); err != nil {
		fmt.Fprintf(w, "{\"resp\": \"err\"}")
		fmt.Println("Err drawing img to inky:", err.Error())
		return
	}
	fmt.Fprintf(w, "{\"resp\": \"ok\"}")
}
