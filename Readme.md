# About this project
So, this is what I needed; 
1. Some sort of terminal, containing a character display and a kepad to accept input of digits and
2. a graphical display, capable of showing QR codes received wirelessly from the terminal. The display needs to be able to be more than the 30 Bluetooth ft apart from the terminal.

This is what I had access to;
* A couple of Raspberry Pis, among them one Raspberry Pi model 3 (RPi3), and one Raspberry Pi Zero W (RPiW)
* A 16 x 2 character no-name LCD display, recovered from an old coffee machine
* A 3 x 4 keys matrix keypad, recovered from an old [Diavox phone](https://www.ericsson.com/en/about-us/history/products/the-telephones/diavox--a-keypad-phone-for-axe)
* A [Pimoroni Inky wHAT eInk display](https://shop.pimoroni.com/products/inky-what?variant=21214020436051)
* An asortment of ATTiny and ATMega AVR chips, and
* a 3D printer.

Easy peasy... :)

## The use case
The reason for this project is to simplify Swish payments and BankID authentications for customers visiting stores here in Sweden. The basic use case is this:
* The cashier enters the amount to pay on the terminal (at the same time displayed on the LCD display) and then presses a "send" button
* A QR code, following the Swish reqirements, is displayed to the customer on the graphical display
* The customer starts the Swish app and scans the QR code. The Swish app will then display the correct amount, the receiving phone number and a suggestion for a note. The amount and receiving phone number cannot be changed by the customer, only the suggested note.
* The customer finishes the payment transaction by signing it using BankID

Just as a proof of concept, I decided to also implement a rudimentary BankID authentication use case:
* The cashier presses the BankID-button on the terminal
* A BankID QR code is displayed on the graphical display
* The customer starts the BankID app, scans the QR code and enters the PIN code/faceID/touchID.
* The user's full name and personal number is displayed to the cashier on the LCD display on the cashier's terminal


## The keypad
Oh, how much I liked the old Diavox phones from the late 80's and 90's! And I love the keypads they used; with their distinct tactile feel to them! I have a few of these old phones laying around, waiting to be part of some future project, and from one of them (broken) I salvaged a keypad.

The keypad is basically a standard 4 row by 3 column matrix display with an additional 8:th wire, allowing for different ways of determining which button was pressed. The ususal (simplest) way is to continuosly poll each of the columns, and when contact is detected; go through the rows to determine which button was pressed.

Using the RPi3 for such a polling wouldn't be optimal though. It's not featuring a real time OS and it would be spending a lot of time in an active loop just doing the polling, potentially slowing other operations down. Another way of solving it would be to use interrupts. But having interrupts attached to three pins (one for each column) didn't appeal to me either. So after a bit of thought I determined to use an ATTiny84 doing the polling of the keypad, and when a key is pressed signal the RPi3, allowing for the RPi3 to retrieve information about which key was pressed.

## The ATTiny84
The ATTiny84 has 12 I/O pins, whereof 7 are used for connecting with the keypad. I opted for using 4 pins as binary data output, leaving 1 I/O pin for signaling the RPi3 when a key is pressed. One of the 12 available pins on the ATTiny84 is the reset pin, but it can be used as a I/O pin if the reset is disabled through setting the RSTDISBL bit in the high fuse (see the [AVR fuse calculator](https://eleccelerator.com/fusecalc/fusecalc.php?chip=attiny84)). Needless to say this needs to be done as the final step (the ATTiny cannot be reprogrammed if reset is disabled) but should one be sloppy enough to disable reset prematurely then a high voltage AVR programmer needs to be used. (And yes, I coincidentally needed to build one of those... :) ).

I'm using the suitable [Arduiono keypad library](https://playground.arduino.cc/Code/Keypad/) to use in the ATTiny84 for interfacing with matrix keypads - saving me time to try to implement it myself.

The implementation on the RPi3 is based on the DetectEdge functionality in the [go-rpi](https://www..com) library. So that when the ATTiny84 had detects a key press it sets the 4 data output pins corresponding to the key that was pressed, and pulls one pin low. This is detected by the RPi3 which reads out the data in parallell from the ATTiny84, allowing for fast detection of key presses and fast data transfer.

Beside the keypad, I also needed two ordinary push buttons on the terminal, and when testing the implementation I got some strange behavior... There seems to be an issue when not using debounced buttons (generating a lot of interrupts) that may cause freezes. I therefore want to point out the workaround explained in the [discussion thread](https://github.com/stianeikeland/go-rpio/issues/35) which can be used if no other application is dependent on interrupts. 

## The LCD display

One may think that all 16 x 2 LCD displays that look the same are created the same. They are not. Besides the fact that some are equiped with a chip allowing for I2C (or SPI) communication, seemingly identical displays may use different LCD driver chips (requiring e.g. different timing), and supporting different character sets. So after salvaging one such display from an old coffee machine and not knowing anything about it, a bit of detective work (and/or plain old trial and error) is required.

After a bit of searching I found out that my particular LCD display was controlled by an ST7066U chip, more or less a clone of the HD44780. I didn't have any I2C driver chip, I wanted to control it using as few pins as possible, and I wanted to control it using Go. I admit I didn't spend too much time searching, but I couldn't find any Go library for this specific driver chip, so I decided to build one. I implemented barely more than what I needed for my project - and I have not implemented a correct mapping to all displayable characters - but you are welcome to download, modify and use it as you like if you find any use for it. It's over at [my Github page](https://github.com/hossner/go-st7066u).

Added note: I suspect that the character set implemented in my display is a Japanese character set called "7066-0A" (as mapped out in this [Sitronix display datasheet](https://www.newhavendisplay.com/app_notes/ST7066U.pdf)), but with a few deviations of course (sigh!).

## The Raspberry Pi 3

Now being able to read the keypad (via the ATTiny84) and presenting the digits on the LCD display, I generate a QR code (as a 300 x 400 pixel PNG) using the [go-qrcode library](https://github.com/skip2/go-qrcode) and transfer it over WiFi to the RPiW, which in turn is connected to the eInk display.

The application is built in Go as a simple state machine, moving between 4 states/modes, and using two go routines for controlling the keypad and the LCD display respectively. The Swhish functionality is rather simple as it is soly based on locally generated QR codes. For the BankID functionality, requiring back-end communication whith the BankID server, I used a previous project of mine where I implemented a [client wrapper around the BankID v5.1 appapi interface](https://github.com/hossner/bankid).

## The Pimoroni Inky wHAT eInk display

I got this display (the black/white/red version) as a gift a few years ago, but as much as I appreciated it I really could find no good use for it for a while. I think eInk displays are really cool, so I was very glad to be able to use it for something useful as part of this project.

As it is designed as a Raspberry Pi "hat", the display basically hogs all the RPiW's GPIO pins (yes, I know there are different ways of breaking out the pins should one whant to). In this project I have no use for the pins not used by the display, but I first thought I'd try to make the whole unit (containing the RPiW and the eInk display) as small as possible, and spent some time playing with the idea of not connecting the display directly to the RPiW's GPIO header. But lazy as I am, and realizing that would be more trouble than neccessary, i ended up connecting the two parts as intended and just printed the box for it a little larger.

## The Raspberry Pi Zero W

Thanks tp [periph.io](https://periph.io/) there's an [experimental support](https://pkg.go.dev/periph.io/x/periph/experimental/devices/inky) for Pimoroni's eInk displays pHAT and wHAT, meaning I could continue using my preferred language :) I noticed no problems using the experimental periph library, but then again; I'm not using much of the functionality.

The implementation on the RPiW is very simple; a rudimentary HTTP handler that decodes the POST:ed data from the RPi3 and draws the received image to the eInk display.

# Next steps
Well, in this case there will not be much of any next steps. This was done as a "quick and dirty" proof of concept and it will be tested out in a flower shop in Sweden to see if it's something customers find useful. Other than that I have no plans for it.

Hope you found anything from this project useful in some of your own projects!