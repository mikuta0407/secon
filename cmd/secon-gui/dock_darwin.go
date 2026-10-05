package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// メニューバー常駐アプリとして Dock にアイコンを出さない
static void seconHideDock(void) {
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// Dock に出ないアプリはウィンドウを開いても前面に来ないので明示的に前面化する
static void seconActivate(void) {
	[NSApp activateIgnoringOtherApps:YES];
}
*/
import "C"

func hideDock() { C.seconHideDock() }
func activate() { C.seconActivate() }
