package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hossner/bankid"
	"github.com/hossner/go-st7066u"
	"github.com/skip2/go-qrcode"
	"github.com/stianeikeland/go-rpio"
)

const configFileName string = "qr-service.cfg"

// The different modes of the display
const (
	swishMode = iota
	bankidMode
	settingsMode
	errorMode
)

// Commands used in the displayCmd
const (
	lcdPrintByte = iota
	lcdClear
	lcdPrintAt
	lcdMoveTo
	lcdMoveLeft
	lcdCursorShow
	lcdCursorHide
)

// Cursor postion in Swish mode
const (
	swishCurPosX uint8 = 12
	swishCurPosY uint8 = 1
)

// Global configuration
type config struct {
	swishPhoneNr     string
	eInkServerIPAddr string
	eInkServerPort   string
	eInkWidth        int
	eInkHeight       int
	swishMessage     string
	swishMask        string
}

// JSON encoded data to eInk display
type einkStruct struct {
	Cmd  string `json:"cmd"`
	Data []byte `json:"data"`
}

// Command data sent to command the LCD display
type displayCmd struct {
	text            string
	cmd, posX, posY uint8
	char            byte
}

// Command data to send to eInk display
// type einkCmd struct {
// 	data []byte
// }

// State info for swishMode
type swishState struct {
	amnt uint16 // Entered amount
	pos  uint8  // Cursor position info
}

// State info for bankidMode
type bankidState struct {
	conn      *bankid.Connection
	available bool
}

type errInfo struct {
	errLCDMsg string
	errNr     byte
	errMsg    string
}

// Global, ugly, variables
var (
	mainMode byte = swishMode
	oldMode  byte = 255
	einkBusy bool
	qrErr    errInfo
)

func main() {
	cfg, err := getConfig(configFileName)
	if err != nil {
		fmt.Println("Error getting config:", err.Error())
		os.Exit(1)
	}

	if err := rpio.Open(); err != nil {
		fmt.Println("Error opening RPIO:", err.Error())
		os.Exit(1)
	}
	defer rpio.Close()

	displayQueue := make(chan displayCmd, 10)
	keypadQueue := make(chan uint8)

	go display(displayQueue)
	go keypad(keypadQueue)

	var swishS = &swishState{}
	var bidS = &bankidState{}

	for {
		switch mainMode {
		case swishMode:
			runSwishMode(cfg, keypadQueue, displayQueue, swishS)
		case bankidMode:
			runBankidMode(cfg, keypadQueue, displayQueue, bidS)
		case settingsMode:
			runSettingsMode(cfg, keypadQueue, displayQueue)
		default:
			runErrorMode(keypadQueue, displayQueue)
		}
	}
}

// =============== Mode handling functions ===============
func runSwishMode(cfg *config, keypad chan uint8, disp chan displayCmd, state *swishState) {
	if oldMode != mainMode {
		oldMode = mainMode
		showDisplaySwish(disp, state.amnt, state.pos)
	}

	key := <-keypad
	switch key {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9: // A number key was pressed
		if state.amnt > 999 || (key == 0 && state.pos == 0) { // Do nothing if amount > 999, or if 0 entered as first key
			return
		}
		state.amnt = state.amnt*10 + uint16(key)
		disp <- displayCmd{cmd: lcdPrintAt, posY: swishCurPosY, posX: swishCurPosX - state.pos, text: strconv.Itoa(int(state.amnt))}
		disp <- displayCmd{cmd: lcdMoveLeft}
		state.pos++

	case 10: // Send, or '#', key pressed
		if state.amnt == 0 && state.pos == 0 {
			return
		}
		if einkBusy {
			showDisplayBusyW(disp)
			showDisplaySwish(disp, state.amnt, state.pos)
			return
		}
		// Maybe should einkBusy = true here...
		showDisplayWait(disp)
		img, err := getSwishImage(cfg, state.amnt)
		// ... and einkBusy = false here
		if err != nil {
			qrErr = errInfo{errNr: 1, errMsg: "Error getting swish image: " + err.Error(), errLCDMsg: "Error 1"}
			mainMode = errorMode
			return
		}
		go sendImgToEink(cfg, img)
		state.amnt = 0
		state.pos = 0
		showDisplayScanW(disp)
		showDisplaySwish(disp, state.amnt, state.pos)

	case 11: // Clear, or '*', key pressed
		state.amnt = 0
		state.pos = 0
		showDisplaySwish(disp, state.amnt, state.pos)

	case 12, 13: // BankID or settings key pressed
		if einkBusy {
			showDisplayBusyW(disp)
			showDisplaySwish(disp, state.amnt, state.pos)
			return
		}
		state.amnt = 0
		state.pos = 0
		if key == 12 { // BankID key pressed
			mainMode = bankidMode
			return
		}
		mainMode = settingsMode

	default:
		// Out of range command to keypad queue, error signaled
		mainMode = errorMode
		return
	}
}

func runBankidMode(cfg *config, keypad chan uint8, disp chan displayCmd, state *bankidState) {
	oldMode = mainMode
	einkBusy = true
	showDisplayWait(disp)
	disp <- displayCmd{cmd: lcdCursorHide}

	type bidCmd struct {
		ri, msg, txt string
	}
	// bidQueue := make(chan bidCmd)
	bidQueue := make(chan struct{ r, m, d string })

	bc, err := bankid.New("", getCallBackFunc(cfg, bidQueue))
	if err != nil {
		em := [2]string{"Failed connect", "to BankID..."}
		showDisplayErrW(disp, em)
		mainMode = swishMode
		return
	}
	defer bc.Close()

	ri := bc.SendRequest(cfg.eInkServerIPAddr, "", "", &bankid.Requirements{TokenStartRequired: true}, nil)
	for {
		stat := <-bidQueue
		if stat.r != ri {
			continue
		}
		switch stat.m {
		case "sent":
			disp <- displayCmd{cmd: lcdClear}
			disp <- displayCmd{cmd: lcdPrintAt, text: "Ask user to scan"}
			disp <- displayCmd{posX: 0, posY: 1, cmd: lcdPrintAt, text: "BankID QR code"}
			img, err := getBankIDImage(cfg, bc, ri)
			if err != nil {
				qrErr = errInfo{errNr: 3, errMsg: "Error getting BankID image: " + err.Error(), errLCDMsg: "Error 3"}
				mainMode = errorMode
				return
			}
			go sendImgToEink(cfg, img)
		case "outstandingTransaction":
		case "started":
		case "userSign":
		case "failed":
			em := [2]string{"Identification"}
			if stat.d == "expiredTransaction" {
				em[1] = "failed. Timeout?"
			} else {
				em[1] = "failed. Aborted?"
			}
			showDisplayErrW(disp, em)
			mainMode = swishMode
			return
		case "complete":
			nm := strings.Split(stat.d, "\n")
			disp <- displayCmd{cmd: lcdClear}
			disp <- displayCmd{cmd: lcdPrintAt, text: nm[0]}
			disp <- displayCmd{posX: 0, posY: 1, cmd: lcdPrintAt, text: nm[1]}
			<-keypad
			mainMode = swishMode
			return
		}
	}
}

func runSettingsMode(cfg *config, keypad chan uint8, disp chan displayCmd) {
	if oldMode != mainMode {
		oldMode = mainMode
	}
	showDisplayErrW(disp, [2]string{"Function not", "implemented yet"})
	mainMode = swishMode
}

func runErrorMode(keypad chan uint8, disp chan displayCmd) {
	if oldMode != mainMode {
		oldMode = mainMode
		log.Println(qrErr.errNr, qrErr.errMsg)
		disp <- displayCmd{cmd: lcdClear}
		disp <- displayCmd{cmd: lcdPrintAt, text: qrErr.errLCDMsg}
		disp <- displayCmd{cmd: lcdPrintAt, posX: 0, posY: 1, text: "Please restart!"}
	}
	<-keypad
}

// =============== Aux functions ===============
func showDisplaySwish(disp chan displayCmd, amnt uint16, pos uint8) {
	if pos > 0 {
		pos--
	}
	disp <- displayCmd{cmd: lcdClear}
	disp <- displayCmd{cmd: lcdPrintAt, text: "Amount (# send)"}
	disp <- displayCmd{cmd: lcdPrintAt, posY: swishCurPosY, posX: swishCurPosX - pos, text: strconv.Itoa(int(amnt)) + ",00"}
	disp <- displayCmd{cmd: lcdMoveTo, posY: swishCurPosY, posX: swishCurPosX}
	disp <- displayCmd{cmd: lcdCursorShow}
}

func showDisplayWait(disp chan displayCmd) {
	disp <- displayCmd{cmd: lcdClear}
	disp <- displayCmd{cmd: lcdPrintAt, text: "    Wait..."}
}

func showDisplayBusyW(disp chan displayCmd) {
	disp <- displayCmd{cmd: lcdClear}
	disp <- displayCmd{cmd: lcdPrintAt, text: "Display is busy"}
	disp <- displayCmd{cmd: lcdPrintAt, posY: 1, posX: 0, text: "please wait..."}
	time.Sleep(time.Millisecond * 2000)
}

func showDisplayScanW(disp chan displayCmd) {
	disp <- displayCmd{cmd: lcdClear}
	disp <- displayCmd{cmd: lcdPrintAt, text: "Ask customer to"}
	disp <- displayCmd{cmd: lcdPrintAt, posX: 0, posY: 1, text: "scan the QR code"}
	time.Sleep(time.Millisecond * 4000)
}

func showDisplayErrW(disp chan displayCmd, msg [2]string) {
	disp <- displayCmd{cmd: lcdCursorHide}
	disp <- displayCmd{cmd: lcdClear}
	disp <- displayCmd{cmd: lcdPrintAt, text: msg[0]}
	disp <- displayCmd{cmd: lcdPrintAt, posY: 1, posX: 0, text: msg[1]}
	time.Sleep(time.Millisecond * 4000)
}

func sendImgToEink(cfg *config, buf *bytes.Buffer) {
	einkBusy = true

	bdy, err := json.Marshal(einkStruct{Cmd: "swish", Data: buf.Bytes()})
	if err != nil {
		qrErr = errInfo{errNr: 2, errMsg: "Failed to JSON encode data to eInk display: " + err.Error(), errLCDMsg: "Error 3"}
		mainMode = errorMode
		return
	}

	resp, err := http.Post("http://"+cfg.eInkServerIPAddr+":"+cfg.eInkServerPort+"/", "application/json", bytes.NewBuffer(bdy))
	if err != nil {
		qrErr = errInfo{errNr: 2, errMsg: "Failed to send request to " + cfg.eInkServerIPAddr + ":" + cfg.eInkServerPort + " " + err.Error(), errLCDMsg: "Error 3"}
		mainMode = errorMode
		return
	}
	defer resp.Body.Close()

	bdy, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		qrErr = errInfo{errNr: 2, errMsg: "Failed parsing response from " + cfg.eInkServerIPAddr + ":" + cfg.eInkServerPort + " " + err.Error(), errLCDMsg: "Error 3"}
		mainMode = errorMode
		return
	}
	//fmt.Println("Response:", string(bdy))

	einkBusy = false
}

func getConfig(nf string) (*config, error) {
	// TODO: Get this from file instead
	c := &config{
		swishPhoneNr:     "9008095",
		eInkServerIPAddr: "192.168.1.186",
		eInkServerPort:   "7760",
		eInkWidth:        400,
		eInkHeight:       300,
		swishMessage:     "Röda korset",
		swishMask:        "4",
	}
	return c, nil
}

func getCallBackFunc(cfg *config, bidQ chan struct{ r, m, d string }) func(string, string, string) {
	return func(reqID, msg, detail string) {
		bidQ <- struct{ r, m, d string }{reqID, msg, detail}
	}
}

// =============== QR code generating funcs ===============
func getSwishImage(cfg *config, amnt uint16) (*bytes.Buffer, error) {
	backgroundImg := image.NewRGBA(image.Rect(0, 0, cfg.eInkWidth, cfg.eInkHeight))
	for x := 0; x < cfg.eInkWidth; x++ {
		for y := 0; y < cfg.eInkHeight; y++ {
			backgroundImg.Set(x, y, color.White)
		}
	}

	qrBuf, err := qrcode.Encode("C"+cfg.swishPhoneNr+";"+strconv.Itoa(int(amnt))+";"+url.PathEscape(cfg.swishMessage)+";"+cfg.swishMask, qrcode.Low, cfg.eInkHeight)
	if err != nil {
		mainMode = errorMode
		return nil, errors.New("Failed to generate QR code: " + err.Error())
	}

	qrImg, _, err := image.Decode(bytes.NewReader(qrBuf))
	if err != nil {
		mainMode = errorMode
		return nil, errors.New("Err 2: Failed to decode QR image buffer: " + err.Error())
	}
	qrImgBounds := qrImg.Bounds()

	imagePoint := image.Point{X: 50, Y: 0}
	imageRect := image.Rectangle{imagePoint, imagePoint.Add(qrImgBounds.Size())}
	draw.Draw(backgroundImg, imageRect, qrImg, qrImgBounds.Min, draw.Src)

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, backgroundImg); err != nil {
		mainMode = errorMode
		return nil, errors.New("Failed to encode image: " + err.Error())
	}

	return buf, nil
}

func getBankIDImage(cfg *config, bc *bankid.Connection, ri string) (*bytes.Buffer, error) {
	backgroundImg := image.NewRGBA(image.Rect(0, 0, cfg.eInkWidth, cfg.eInkHeight))
	for x := 0; x < cfg.eInkWidth; x++ {
		for y := 0; y < cfg.eInkHeight; y++ {
			backgroundImg.Set(x, y, color.White)
		}
	}

	qrBuf, err := bc.GenerateQRCode(ri, cfg.eInkHeight)
	if err != nil {
		mainMode = errorMode
		return nil, errors.New("Failed to generate QR code: " + err.Error())
	}

	qrImg, _, err := image.Decode(bytes.NewReader(qrBuf))
	if err != nil {
		mainMode = errorMode
		return nil, errors.New("Err 2: Failed to decode QR image buffer: " + err.Error())
	}
	qrImgBounds := qrImg.Bounds()

	imagePoint := image.Point{X: 50, Y: 0}
	imageRect := image.Rectangle{imagePoint, imagePoint.Add(qrImgBounds.Size())}
	draw.Draw(backgroundImg, imageRect, qrImg, qrImgBounds.Min, draw.Src)

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, backgroundImg); err != nil {
		mainMode = errorMode
		return nil, errors.New("Failed to encode image: " + err.Error())
	}

	return buf, nil
}

// =============== Go routines handling peripherals ===============
func display(queue chan displayCmd) {
	led := rpio.Pin(26)
	led.Output()

	lcd, err := st7066u.New(2, 16, st7066u.DOTS5x8, st7066u.BITMODE4, 7, 8, 15, 25, 24, 23, 18)
	if err != nil {
		qrErr = errInfo{errNr: 2, errMsg: "Failed opening LCD display: " + err.Error(), errLCDMsg: "Error 3"}
		mainMode = errorMode
		return
	}
	lcd.LedOn(true)

	for {
		st := <-queue
		switch st.cmd {
		case lcdPrintByte: // Print a single byte
			lcd.PrintByte(st.char)

		case lcdClear: // Clear
			lcd.Clear()

		case lcdPrintAt: // Print a string
			lcd.PrintAt(st.posY, st.posX, st.text)

		case lcdMoveTo: // Move cursor to pos
			lcd.SetCursor(st.posY, st.posX)

		case lcdCursorShow: // Show cursor
			lcd.CursorOn(true)

		case lcdCursorHide: // Hide cursor
			lcd.CursorOn(false)

		case lcdMoveLeft: // Hide cursor
			lcd.MoveLeft(1)

		default:
			// Unknown command to LCD; ignore it
		}
	}
}

func keypad(queue chan uint8) {
	inputs := [4]rpio.Pin{12, 16, 20, 21}
	for i := 0; i < 4; i++ {
		inputs[i].Input()
		inputs[i].PullDown()
	}
	keypadPin := rpio.Pin(22) // Select-pin from ATTiny
	keypadPin.Input()
	keypadPin.PullUp()
	keypadPin.Detect(rpio.FallEdge)

	bankidPin := rpio.Pin(6) // Pin from BankID-button
	bankidPin.Input()
	bankidPin.PullUp()
	bankidPin.Detect(rpio.FallEdge)

	settingsPin := rpio.Pin(5) // Pin from settings-button
	settingsPin.Input()
	settingsPin.PullUp()
	settingsPin.Detect(rpio.FallEdge)

	defer keypadPin.Detect(rpio.NoEdge)
	defer bankidPin.Detect(rpio.NoEdge)
	defer settingsPin.Detect(rpio.NoEdge)

	var ch uint8
	for { // Interrupts disabled due to bug, active polling instead :(
		if keypadPin.EdgeDetected() {
			for p := 3; p >= 0; p-- {
				ch |= uint8(rpio.ReadPin(inputs[p])) << (3 - p)
			}
			queue <- ch
			ch = 0
			time.Sleep(time.Millisecond * 2)
		}

		if bankidPin.EdgeDetected() {
			queue <- 12
		}

		if settingsPin.EdgeDetected() {
			queue <- 13
		}
	}
}
