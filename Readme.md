This is what I needed; 1) a terminal of sorts, on which you use a kepad to enter digits and 2) a display capable of showing QR codes containing the digits that were entered on the terminal. And the QR code display also needs to be able to be more than the 30 Bluetooth ft apart from the terminal.

This is what I had; a couple of Raspberry Pi:s from different generations, a 16 x 2 LCD display from an old coffee machine, a keypad from an old Diavox phone, an Inky WHat display received as a present a few years ago, an asortment of ATTiny and ATMega AVR chips and a 3D printer. Easy peasy... :)


## The keypad

Oh, how I liked the old Diavox phones from the late 80's and 90's! And I love the keypad they used; with a distinct tactile feel and great feeling! I have a few of these old phones laying around, waiting to be part of some future project, and from one of them (broken) I salvaged a keypad. I really didn't want there to be a delay between entering the digits on the keypad and them showing up on the LCD display. The keypad as basically a standard 4 row, 3 column matrix display with an additional 8:th wire, allowing for different ways of reading which button was pressed. And knowing that the Raspberry Pi (model 3) would be busy as it is, I didn't want the Pi to have to poll the wires from the keypad. If possible I'd like an interrupt based way of solving it. So after a bit of thought I settled for using an ATTiny84 doing the polling instead. I found a suitable library [here](https://www.www.com) and wired it up. 

## The ATTiny84

The ATTiny 84 has 12 I/O pins and besides the 7 wires from the keypad, I opted for using 4 wires as data output, and 1 as signal/select pin to the Pi. One of the 12 available pins on the ATTiny85 is the reset pin, but can be used as a I/O pin if the reset is disabled by setting the fuse.

This setup allows for the Pi to use the DetectEdge functionality (in the [go-rpi](https://www.www.com) library and, when the ATTiny84 had read the keypad, read out the data in parallell from the 4 data pins, allowing for fast detection and fast data transfer from the ATTiny84.

## The LCD display

After a bit of searching I found out that the LCD display was controlled by an ST7066U chip, more or less a clone of the HD44780. I didn't have any I2C driver chip, wanted to drive it using as few wires as possible, and wanted to control it using Go. I admit I didn't spend too much time searching, but I couldn't find ay Go library for this display, so I decided to build one. I implemented barely more than what I needed for my project, but should you have any use of it you are welcome to download, modify and use it as you like from [the Github page](https://github.com/hossner/gopilcd).

## The RaspberryPi 3

Now being able to read the keypad and presenting the digits on the LCD display, I generate a QR code (as a 300 x 400 pixel PNG) and transfer it over WiFi to the RaspberryPi model B.
