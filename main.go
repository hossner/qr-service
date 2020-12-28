package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hossner/gopilcd"
	"github.com/stianeikeland/go-rpio"
)

// https://github.com/farhan0syakir/go-pad4pi/blob/master/pad4pi.go

type displayCmd struct {
	text            string
	cmd, posX, posY uint8
	char            byte
}

func main() {
	// 1. Read in config
	// 2. Start LCD display Go routine
	// 3. Start network Go routine
	// 4. Connect to the QR code display
	// 5. Start key pad Go routine
	// 6. Listen to input from key pad

	if err := rpio.Open(); err != nil {
		fmt.Println("main:", err)
		os.Exit(1)
	}
	// Unmap gpio memory when done
	defer rpio.Close()

	displayQueue := make(chan displayCmd, 10)
	keypadQueue := make(chan uint8)

	go display(displayQueue)
	go keypad(keypadQueue)

	for {
		key := <-keypadQueue
		if key < 10 {
			// Here: Check what key was pressed and set displayCmd.cmd accordingly
			// fmt.Println("From keypad:", key)
			//fmt.Printf("%0.8b = %d = %s\n", key, key, string(key+'0'))
			displayQueue <- displayCmd{char: key + '0'}
			continue
		}
		if key == 11 { // Clear (or *) button pressed
			displayQueue <- displayCmd{cmd: 1}
			continue
		}
		if key == 10 { // Send button pressed
			displayQueue <- displayCmd{cmd: 1} // Clear LCD display
			displayQueue <- displayCmd{cmd: 2, text: "Ask customer to"}
			displayQueue <- displayCmd{cmd: 2, posY: 1, posX: 0, text: "scan the QR code"}
			time.Sleep(time.Millisecond * 2000)
			displayQueue <- displayCmd{cmd: 1}
		}
	}
}

func display(queue chan displayCmd) {
	led := rpio.Pin(26)
	led.Output()

	lcd, err := gopilcd.New(2, 16, gopilcd.DOTS5x8, gopilcd.BITMODE4, 7, 8, 15, 25, 24, 23, 18)
	if err != nil {
		log.Fatalln(err)
	}
	lcd.LedOn(true)

	for {
		st := <-queue
		if st.cmd == 0 {
			lcd.PrintByte(st.char)
			continue
			// led.High()
			// time.Sleep(time.Millisecond * 20)
			// led.Low()
		}
		if st.cmd == 1 { // Clear
			lcd.Clear()
		}
		if st.cmd == 2 { // Print a string
			lcd.PrintAt(st.posY, st.posX, st.text)
		}
	}
}

func keypad(queue chan uint8) {
	inputs := [4]rpio.Pin{12, 16, 20, 21}
	for i := 0; i < 4; i++ {
		inputs[i].Input()
		inputs[i].PullDown()
	}
	// Use mcu pin 22, corresponds to GPIO 3 on the pi
	pin := rpio.Pin(22)
	pin.Input()
	pin.PullUp()
	pin.Detect(rpio.FallEdge) // enable falling edge event detection

	fmt.Println("keypad: press a button")

	var ch uint8
	for {
		if pin.EdgeDetected() { // check if event occured
			// Read input pins from keypad and store in uint8, then put on queue
			for p := 3; p >= 0; p-- {
				ch |= uint8(rpio.ReadPin(inputs[p])) << (3 - p)
			}
			queue <- ch
			ch = 0
			// Wait to allow edge to fall again. Can be set to 1 after more code?
			time.Sleep(time.Millisecond * 2)
		}
	}
	pin.Detect(rpio.NoEdge) // disable edge event detection
}
