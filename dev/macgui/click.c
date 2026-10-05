// click X Y [move]  : 指定座標 (ポイント) を左クリックする (move なら移動のみ)
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
