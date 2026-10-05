// click X Y [move]  : 指定座標 (ポイント) を左クリックする (move なら移動のみ)
// click right X Y    : 右クリック
// click double X Y   : ダブルクリック
// click drag X1 Y1 X2 Y2 : (X1,Y1) から (X2,Y2) へドラッグする (ウィンドウのタイトルバーを掴んで移動など)
// click paste        : フォーカス中の欄を全選択してクリップボードの内容を貼り付ける (⌘A ⌘V)
//                      文字入力イベントは GLFW に届かないことがあるので、入力は pbcopy + paste で行う
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
static void key(CGKeyCode code, CGEventFlags flags) {
  CGEventRef d = CGEventCreateKeyboardEvent(NULL, code, true), u = CGEventCreateKeyboardEvent(NULL, code, false);
  CGEventSetFlags(d, flags); CGEventSetFlags(u, flags);
  CGEventPost(kCGHIDEventTap, d); CGEventPost(kCGHIDEventTap, u); usleep(30000);
}

int main(int argc, char **argv) {
  if (argc > 3 && !strcmp(argv[1], "right")) {
    CGPoint p = {atof(argv[2]), atof(argv[3])};
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, p, kCGMouseButtonLeft)); usleep(100000);
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventRightMouseDown, p, kCGMouseButtonRight)); usleep(50000);
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventRightMouseUp, p, kCGMouseButtonRight));
    return 0;
  }
  if (argc > 3 && !strcmp(argv[1], "double")) {
    CGPoint p = {atof(argv[2]), atof(argv[3])};
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, p, kCGMouseButtonLeft)); usleep(100000);
    for (int n = 1; n <= 2; n++) {
      CGEventRef d = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, p, kCGMouseButtonLeft);
      CGEventRef u = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, p, kCGMouseButtonLeft);
      CGEventSetIntegerValueField(d, kCGMouseEventClickState, n);
      CGEventSetIntegerValueField(u, kCGMouseEventClickState, n);
      CGEventPost(kCGHIDEventTap, d); usleep(30000); CGEventPost(kCGHIDEventTap, u); usleep(80000);
    }
    return 0;
  }
  if (argc > 5 && !strcmp(argv[1], "drag")) {
    CGPoint a = {atof(argv[2]), atof(argv[3])}, b = {atof(argv[4]), atof(argv[5])};
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, a, kCGMouseButtonLeft)); usleep(100000);
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, a, kCGMouseButtonLeft)); usleep(100000);
    for (int i = 1; i <= 20; i++) {
      CGPoint p = {a.x + (b.x - a.x) * i / 20, a.y + (b.y - a.y) * i / 20};
      CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDragged, p, kCGMouseButtonLeft)); usleep(20000);
    }
    CGEventPost(kCGHIDEventTap, CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, b, kCGMouseButtonLeft));
    return 0;
  }
  if (argc > 1 && !strcmp(argv[1], "paste")) {
    key(0 /* a */, kCGEventFlagMaskCommand);
    key(9 /* v */, kCGEventFlagMaskCommand);
    return 0;
  }
  CGPoint p = {atof(argv[1]), atof(argv[2])};
  CGEventRef m = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, p, kCGMouseButtonLeft);
  CGEventPost(kCGHIDEventTap, m); usleep(100000);
  if (argc > 3) return 0; // "move" 指定時は移動のみ
  CGEventRef d = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, p, kCGMouseButtonLeft);
  CGEventRef u = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, p, kCGMouseButtonLeft);
  CGEventPost(kCGHIDEventTap, d); usleep(50000); CGEventPost(kCGHIDEventTap, u);
  printf("trusted=%d\n", AXIsProcessTrusted());
  return 0;
}
