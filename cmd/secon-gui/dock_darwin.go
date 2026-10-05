package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
void seconHideDock(void);
void seconActivate(void);
void seconInstallReopenHandler(void);
*/
import "C"

// onReopen は起動中にアプリをもう一度開いたとき (Launchpad・Finder など) に呼ばれる。
var onReopen func()

//export seconReopen
func seconReopen() {
	if onReopen != nil {
		onReopen()
	}
}

func hideDock() {
	C.seconHideDock()
	C.seconInstallReopenHandler()
}

func activate() { C.seconActivate() }
