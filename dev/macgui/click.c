// click X Y : 指定座標 (ポイント) を左クリックする
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>
#include <unistd.h>
int main(int argc, char **argv) {
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
