// Port of Adafruit's "pyportal_boing" demo found here:
// https://github.com/adafruit/Adafruit_ILI9341/blob/master/examples/pyportal_boing
package main

import (
	"device/sam"
	"machine"
	"time"

	"tinygo.org/x/drivers/examples/ili9341/pyportal_boing/graphics"
	"tinygo.org/x/drivers/ili9341"
)

const (
	// invert RGB565
	BGCOLOR    = 0x75AD
	GRIDCOLOR  = 0x15A8
	BGSHADOW   = 0x8552
	GRIDSHADOW = 0x0C60
	RED        = 0x00F8
	WHITE      = 0xFFFF

	YBOTTOM = 123  // Ball Y coord at bottom
	YBOUNCE = -3.5 // Upward velocity on ball bounce

	_debug = false
)

var (
	frameBuffer = [2][(graphics.BALLHEIGHT + 8) * (graphics.BALLWIDTH + 8)]uint16{}
	fi          = 0

	startTime int64
	frame     int64

	// Ball coordinates are stored floating-point because screen refresh
	// is so quick, whole-pixel movements are just too fast!
	ballx     float32
	bally     float32
	ballvx    float32
	ballvy    float32
	ballframe float32
	balloldx  float32
	balloldy  float32

	// Color table for ball rotation effect
	palette      [16]uint16
	paletteCache [36][16]uint16
	cached       = false
	idx          = 0
)

func main() {

	// configure backlight
	backlight.Configure(machine.PinConfig{machine.PinOutput})

	// configure display
	display.Configure(ili9341.Config{})
	print("width, height == ")
	width, height := display.Size()
	println(width, height)

	backlight.High()

	display.SetRotation(ili9341.Rotation270)
	DrawBackground()

	startTime = time.Now().UnixNano()
	frame = 0

	ballx = 20.0
	bally = YBOTTOM // Current ball position
	ballvx = 0.8
	ballvy = YBOUNCE // Ball velocity
	ballframe = 3    // Ball animation frame #
	balloldx = ballx
	balloldy = bally // Prior ball position

	machine.LCD_SS_PIN.Low()
	machine.SPI3.Bus.CTRLB.ClearBits(sam.SERCOM_SPIS_CTRLB_RXEN)
	for {

		balloldx = ballx // Save prior position
		balloldy = bally
		ballx += ballvx // Update position
		bally += ballvy
		ballvy += 0.06 // Update Y velocity
		if (ballx <= 15) || (ballx >= graphics.SCREENWIDTH-graphics.BALLWIDTH) {
			ballvx *= -1 // Left/right bounce
		}
		if bally >= YBOTTOM { // Hit ground?
			bally = YBOTTOM  // Clip and
			ballvy = YBOUNCE // bounce up
		}

		// Determine screen area to update.  This is the bounds of the ball's
		// prior and current positions, so the old ball is fully erased and new
		// ball is fully drawn.
		var minx, miny, maxx, maxy, width, height int16

		// Determine bounds of prior and new positions
		minx = int16(ballx)
		if int16(balloldx) < minx {
			minx = int16(balloldx)
		}
		miny = int16(bally)
		if int16(balloldy) < miny {
			miny = int16(balloldy)
		}
		maxx = int16(ballx + graphics.BALLWIDTH - 1)
		if int16(balloldx+graphics.BALLWIDTH-1) > maxx {
			maxx = int16(balloldx + graphics.BALLWIDTH - 1)
		}
		maxy = int16(bally + graphics.BALLHEIGHT - 1)
		if int16(balloldy+graphics.BALLHEIGHT-1) > maxy {
			maxy = int16(balloldy + graphics.BALLHEIGHT - 1)
		}

		width = maxx - minx + 1
		height = maxy - miny + 1

		if !cached {
			// Ball animation frame # is incremented opposite the ball's X velocity
			ballframe -= ballvx * 0.5
			if ballframe < 0 {
				ballframe += 14 // Constrain from 0 to 13
			} else if ballframe >= 14 {
				ballframe -= 14
			}

			// Set 7 palette entries to white, 7 to red, based on frame number.
			// This makes the ball spin
			for i := 0; i < 14; i++ {
				if (int(ballframe)+i)%14 < 7 {
					paletteCache[idx][i+2] = WHITE
				} else {
					paletteCache[idx][i+2] = RED
				} // Palette entries 0 and 1 aren't used (clear and shadow, respectively)
			}
		}

		// Only the changed rectangle is drawn into the 'renderbuf' array...
		var c uint16              //, *destPtr;
		bx := minx - int16(ballx) // X relative to ball bitmap (can be negative)
		by := miny - int16(bally) // Y relative to ball bitmap (can be negative)

		if 0 < ballvx {
			// to right
			for y := 0; y < int(height); y++ { // For each row...
				for x := 0; x < -int(bx); x++ {
					frameBuffer[fi][y*int(width)+x] = graphics.Background[int(miny)+y][int(minx)+x]
				}
			}
		} else {
			// to left
			for y := 0; y < int(height); y++ { // For each row...
				for x := graphics.BALLWIDTH; x < int(width); x++ {
					frameBuffer[fi][y*int(width)+x] = graphics.Background[int(miny)+y][int(minx)+x]
				}
			}
		}

		if 0 < ballvy {
			for y := 0; y < -int(by); y++ { // For each row...
				for x := 0; x < int(width); x++ {
					frameBuffer[fi][y*int(width)+x] = graphics.Background[int(miny)+y][int(minx)+x]
				}
			}
		} else {
			for y := graphics.BALLHEIGHT; y < int(height); y++ { // For each row...
				for x := 0; x < int(width); x++ {
					frameBuffer[fi][y*int(width)+x] = graphics.Background[int(miny)+y][int(minx)+x]
				}
			}
		}

		palette = paletteCache[idx]
		for y := 0; y < int(graphics.BALLHEIGHT); y++ { // For each row...
			for x := 0; x < int(graphics.BALLWIDTH); x++ {
				c = uint16(graphics.Ball[int(y)*int(graphics.BALLWIDTH)+int(x)])
				if c == 0 { // Outside ball - just draw grid
					c = graphics.Background[int(bally)+y][int(ballx)+x]
				} else if c > 1 { // In ball area...
					c = palette[c]
				} else { // In shadow area...
					c = graphics.BackgroundShadow[int(bally)+y][int(ballx)+x]
				}
				frameBuffer[fi][(y-int(by))*int(width)+(x-int(bx))] = c
			}
		}

		//display.DrawRGBBitmap(minx, miny, frameBuffer[fi][:width*height], width, height)
		display.DrawRGBBitmapDMA(minx, miny, frameBuffer[fi][:width*height], width, height)
		//time.Sleep(100 * time.Millisecond)
		//display.DrawRGBBitmapDMA(int16(ballx), int16(bally), frameBuffer[fi][:width*height], width, height)
		fi = 1 - fi

		// Show approximate frame rate
		frame++
		if frame&0x1FF == 0 { // Every 256 frames...
			elapsed := (time.Now().UnixNano() - startTime) / int64(time.Second)
			if elapsed > 0 {
				if true {
					println(frame/elapsed, " fps")
				}
			}
		}

		if 0 < ballvx {
			idx++
			if idx == 36 {
				idx = 0
				cached = true
			}
		} else {
			if 0 < idx {
				idx--
			} else {
				idx = 35
			}
		}

	}
}

func DrawBackground() {
	w, h := display.Size()

	for j := int16(0); j < h; j++ {
		for k := int16(0); k < w; k++ {
			frameBuffer[0][k] = graphics.Background[j][k]
		}
		display.DrawRGBBitmap(0, j, frameBuffer[0][0:w], w, 1)
	}
}
