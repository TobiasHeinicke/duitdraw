package duitdraw

import (
	"image"
	"io"
	"time"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

// EventLoop is the event loop for a single window.
func (d *Display) eventLoop(errch chan<- error) {
	w := d.window
	b := d.buffer
	var err error

	resizeFunc := func(e size.Event) {
		d.ScreenImage.Lock()
		if b != nil {
			b.Release()
		}
		b, err = mainScreen.NewBuffer(e.Size())
		if err != nil {
			errch <- err
			d.ScreenImage.Unlock()
			return
		}
		d.buffer = b
		d.ScreenImage.m = b.RGBA()
		d.ScreenImage.R = b.Bounds()
		d.ScreenImage.Unlock()
		d.mouse.Resize <- true
	}
	// Initial resize call, to allocate the buffer.
	resizeFunc(size.Event{WidthPx: 800, HeightPx: 600})

	// Send an initial mouse event to trigger a Redraw.
	d.mouse.C <- d.mouse.Mouse

	// Delay and filter resize events.
	resize := make(chan size.Event)
	go func(se chan size.Event) {
		var cur size.Event
		delay := 100 * time.Millisecond
		t := time.NewTimer(delay)
		for {
			select {
			case e := <-se:
				cur = e
				t.Reset(delay) // Is that safe?
			case <-t.C:
				resizeFunc(cur)
			}
		}
	}(resize)

	for {
		switch e := w.NextEvent().(type) {
		case lifecycle.Event:
			if e.To == lifecycle.StageDead {
				errch <- io.EOF
				return
			}

		case paint.Event:
			if b != nil {

				d.ScreenImage.Lock()
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
				d.ScreenImage.Unlock()
			}

		case size.Event:
			// When minimizing a window, it receives a size.Event,
			// but the new size is 0. duit complains about and exits.
			if e.WidthPx == 0 {
				continue
			}
			resize <- e

		case mouse.Event:
			// Mouse.Buttons stores a bitmask for each button state.
			// On the other side a mouse.Event arrives, if anything changes.
			if e.Button > 0 { // TODO: wheel is < 0
				if e.Direction == mouse.DirPress {
					// Uncomment for cursorOffset calibration:
					// fmt.Printf("shiny: mouse click: %f %f\n", e.X, e.Y)
					d.mouse.Buttons ^= 1 << uint(e.Button-1)
				} else if e.Direction == mouse.DirRelease {
					d.mouse.Buttons &= ^(1 << uint(e.Button-1))
				}
			} else if e.Button < 0 {
				// For mouse wheel events, we receive a single event
				// but duit expects two: set the bit and release it.
				shift := uint(3) // ButtonWheelUp
				if e.Button == mouse.ButtonWheelDown {
					shift = 4
				}
				d.mouse.Buttons ^= 1 << shift
				d.sendMouseEvent(e)
				d.mouse.Buttons &= ^(1 << shift)
			}
			d.sendMouseEvent(e)

		case key.Event:
			if t := d.KeyTranslator; t == nil {
				// We forward the event for key presses and subsequent events
				// if the key remains down, but not for releases.
				var sendKey rune = -1
				if r := e.Rune; e.Direction != key.DirRelease {
					if r != -1 {
						sendKey = r
					} else {
						if r, ok := keymap[e.Code]; ok {
							sendKey = r
						}
					}

				}
				if sendKey != -1 {
					// Shiny sends \r on Enter, duit expects \n.
					if sendKey == '\r' {
						sendKey = '\n'
					}
					if e.Modifiers == 0x2 { // Ctrl
						if r, ok := ctrlMods[sendKey]; ok {
							sendKey = r
						}
					}
					// fmt.Printf("shiny: key: %x %v\n", sendKey, e)
					d.keyboard.C <- sendKey
				}

				// TODO: what about Shift-KeyLeft/Right
				// to mark text? This seems to be unsupported in duit right now.

			} else if r := t.TranslateKey(e); r != -1 {
				d.keyboard.C <- r
			}
		case error:
			errch <- e

		}
	}
}

func (d *Display) sendMouseEvent(e mouse.Event) {
	d.mouse.Point.X = int(e.X)
	d.mouse.Point.Y = int(e.Y)
	t := time.Now().UnixNano() / (1000 * 1000) // Milliseconds
	d.mouse.Msec = uint32(t)
	d.mouse.C <- d.mouse.Mouse
}
