// 29 march 2014

package ui

import (
	"image"
	"unsafe"
)

//// #include <HIToolbox/Events.h>
// #include "objc_darwin.h"
import "C"

type area struct {
	*areabase

	_id		C.id
	scroller	*scroller
}

func newArea(ab *areabase) Area {
	a := &area{
		areabase:		ab,
	}
	a._id = C.newArea(unsafe.Pointer(a))
	a.scroller = newScroller(a._id)
	a.SetSize(a.width, a.height)
	return a
}

func (a *area) SetSize(width, height int) {
	a.width = width
	a.height = height
	// set the frame size to set the area's effective size on the Cocoa side
	C.moveControl(a._id, 0, 0, C.intptr_t(a.width), C.intptr_t(a.height))
}

func (a *area) RepaintAll() {
	C.areaRepaintAll(a._id)
}

//export areaView_drawRect
func areaView_drawRect(self C.id, rect C.struct_xrect, data unsafe.Pointer) {
	a := (*area)(data)
	// no need to clear the clip rect; the NSScrollView does that for us (see the setDrawsBackground: call in objc_darwin.m)
	// rectangles in Cocoa are origin/size, not point0/point1; if we don't watch for this, weird things will happen when scrolling
	cliprect := image.Rect(int(rect.x), int(rect.y), int(rect.x+rect.width), int(rect.y+rect.height))
	max := C.frame(self)
	cliprect = image.Rect(0, 0, int(max.width), int(max.height)).Intersect(cliprect)
	if cliprect.Empty() { // no intersection; nothing to paint
		return
	}
	i := a.handler.Paint(cliprect)
	C.drawImage(
		unsafe.Pointer(pixelData(i)), C.intptr_t(i.Rect.Dx()), C.intptr_t(i.Rect.Dy()), C.intptr_t(i.Stride),
		C.intptr_t(cliprect.Min.X), C.intptr_t(cliprect.Min.Y))
}

func parseModifiers(e C.id) (m Modifiers) {
	const (
		// TODO define these on the Objective-C side
		_NSShiftKeyMask     = 1 << 17
		_NSControlKeyMask   = 1 << 18
		_NSAlternateKeyMask = 1 << 19
		_NSCommandKeyMask   = 1 << 20
	)

	mods := uintptr(C.modifierFlags(e))
	if (mods & _NSControlKeyMask) != 0 {
		m |= Ctrl
	}
	if (mods & _NSAlternateKeyMask) != 0 {
		m |= Alt
	}
	if (mods & _NSShiftKeyMask) != 0 {
		m |= Shift
	}
	if (mods & _NSCommandKeyMask) != 0 {
		m |= Super
	}
	return m
}

func areaMouseEvent(self C.id, e C.id, click bool, up bool, data unsafe.Pointer) {
	var me MouseEvent

	a := (*area)(data)
	xp := C.getTranslatedEventPoint(self, e)
	me.Pos = image.Pt(int(xp.x), int(xp.y))
	// for the most part, Cocoa won't geenerate an event outside the Area... except when dragging outside the Area, so check for this
	max := C.frame(self)
	if !me.Pos.In(image.Rect(0, 0, int(max.width), int(max.height))) {
		return
	}
	me.Modifiers = parseModifiers(e)
	which := uint(C.buttonNumber(e)) + 1
	if which == 3 { // swap middle and right button numbers
		which = 2
	} else if which == 2 {
		which = 3
	}
	if click && up {
		me.Up = which
	} else if click {
		me.Down = which
		// this already works the way we want it to so nothing special needed like with Windows and GTK+
		me.Count = uint(C.clickCount(e))
	} else {
		which = 0 // reset for Held processing below
	}
	// the docs do say don't use this for tracking (mouseMoved:) since it returns the state now, and mouse move events work by tracking, but as far as I can tell dragging the mouse over the inactive window does not generate an event on Mac OS X, so :/ (tracking doesn't touch dragging anyway except during mouseEntered: and mouseExited:, which we don't handle, and the only other tracking message, cursorChanged:, we also don't handle (yet...? need to figure out if this is how to set custom cursors or not), so)
	held := C.pressedMouseButtons()
	if which != 1 && (held&1) != 0 { // button 1
		me.Held = append(me.Held, 1)
	}
	if which != 2 && (held&4) != 0 { // button 2; mind the swap
		me.Held = append(me.Held, 2)
	}
	if which != 3 && (held&2) != 0 { // button 3
		me.Held = append(me.Held, 3)
	}
	held >>= 3
	for i := uint(4); held != 0; i++ {
		if which != i && (held&1) != 0 {
			me.Held = append(me.Held, i)
		}
		held >>= 1
	}
	a.handler.Mouse(me)
}

//export areaView_mouseMoved_mouseDragged
func areaView_mouseMoved_mouseDragged(self C.id, e C.id, data unsafe.Pointer) {
	// for moving, this is handled by the tracking rect stuff above
	// for dragging, if multiple buttons are held, only one of their xxxMouseDragged: messages will be sent, so this is OK to do
	areaMouseEvent(self, e, false, false, data)
}

//export areaView_mouseDown
func areaView_mouseDown(self C.id, e C.id, data unsafe.Pointer) {
	// no need to manually set focus; Mac OS X has already done that for us by this point since we set our view to be a first responder
	areaMouseEvent(self, e, true, false, data)
}

//export areaView_mouseUp
func areaView_mouseUp(self C.id, e C.id, data unsafe.Pointer) {
	areaMouseEvent(self, e, true, true, data)
}

func sendKeyEvent(self C.id, ke KeyEvent, data unsafe.Pointer) {
	a := (*area)(data)
	a.handler.Key(ke)
}

func areaKeyEvent(self C.id, e C.id, up bool, data unsafe.Pointer) {
	var ke KeyEvent

	keyCode := uintptr(C.keyCode(e))
	ke, ok := fromKeycode(keyCode)
	if !ok {
		// no such key; modifiers by themselves are handled by -[self flagsChanged:]
		return
	}
	// either ke.Key or ke.ExtKey will be set at this point
	ke.Modifiers = parseModifiers(e)
	ke.Up = up
	sendKeyEvent(self, ke, data)
}

//export areaView_keyDown
func areaView_keyDown(self C.id, e C.id, data unsafe.Pointer) {
	areaKeyEvent(self, e, false, data)
}

//export areaView_keyUp
func areaView_keyUp(self C.id, e C.id, data unsafe.Pointer) {
	areaKeyEvent(self, e, true, data)
}

//export areaView_flagsChanged
func areaView_flagsChanged(self C.id, e C.id, data unsafe.Pointer) {
	var ke KeyEvent

	// Mac OS X sends this event on both key up and key down.
	// Fortunately -[e keyCode] IS valid here, so we can simply map from key code to Modifiers, get the value of [e modifierFlags], and check if the respective bit is set or not — that will give us the up/down state
	keyCode := uintptr(C.keyCode(e))
	mod, ok := keycodeModifiers[keyCode] // comma-ok form to avoid adding entries
	if !ok {                             // unknown modifier; ignore
		return
	}
	ke.Modifiers = parseModifiers(e)
	ke.Up = (ke.Modifiers & mod) == 0
	ke.Modifier = mod
	// don't include the modifier in ke.Modifiers
	ke.Modifiers &^= mod
	sendKeyEvent(self, ke, data)
}

func (a *area) id() C.id {
	return a._id
}

func (a *area) setParent(p *controlParent) {
	a.scroller.setParent(p)
}

func (a *area) allocate(x int, y int, width int, height int, d *sizing) []*allocation {
	return baseallocate(a, x, y, width, height, d)
}

func (a *area) preferredSize(d *sizing) (width, height int) {
	// the preferred size of an Area is its size
	return a.width, a.height
}

func (a *area) commitResize(c *allocation, d *sizing) {
	a.scroller.commitResize(c, d)
}

func (a *area) getAuxResizeInfo(d *sizing) {
	basegetAuxResizeInfo(a, d)
}
