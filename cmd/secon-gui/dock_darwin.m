#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include "_cgo_export.h"

// メニューバー常駐アプリとして Dock にアイコンを出さない
void seconHideDock(void) {
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// Dock に出ないアプリはウィンドウを開いても前面に来ないので明示的に前面化する
void seconActivate(void) {
	[NSApp activateIgnoringOtherApps:YES];
}

static BOOL seconShouldHandleReopen(id self, SEL _cmd, NSApplication *app, BOOL hasVisibleWindows) {
	seconReopen();
	return YES;
}

// 起動中のアプリをもう一度開くと AppKit は新しいプロセスを作らず reopen を送ってくる。
// GLFW のデリゲートはこれを実装していないので、実行時にメソッドを追加して受け取る
void seconInstallReopenHandler(void) {
	id delegate = [NSApp delegate];
	if (delegate == nil) {
		return;
	}
	class_addMethod([delegate class], @selector(applicationShouldHandleReopen:hasVisibleWindows:),
	                (IMP)seconShouldHandleReopen, "c@:@c");
}
