// 指定プロセス名のウィンドウ一覧 (ID・位置・サイズ・画面上か) を出す。ID は screencapture -l に渡せる
#include <ApplicationServices/ApplicationServices.h>
#include <stdio.h>
#include <string.h>
int main(int argc, char **argv) {
  CFArrayRef a = CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID);
  for (CFIndex i = 0; i < CFArrayGetCount(a); i++) {
    CFDictionaryRef d = CFArrayGetValueAtIndex(a, i);
    char owner[256] = "", name[256] = "";
    CFStringRef o = CFDictionaryGetValue(d, kCGWindowOwnerName), n = CFDictionaryGetValue(d, kCGWindowName);
    if (o) CFStringGetCString(o, owner, sizeof owner, kCFStringEncodingUTF8);
    if (n) CFStringGetCString(n, name, sizeof name, kCFStringEncodingUTF8);
    if (argc > 1 && !strstr(owner, argv[1])) continue;
    CGRect r; CGRectMakeWithDictionaryRepresentation(CFDictionaryGetValue(d, kCGWindowBounds), &r);
    CFBooleanRef on = CFDictionaryGetValue(d, kCGWindowIsOnscreen);
    int layer = 0, id = 0;
    CFNumberGetValue(CFDictionaryGetValue(d, kCGWindowLayer), kCFNumberIntType, &layer);
    CFNumberGetValue(CFDictionaryGetValue(d, kCGWindowNumber), kCFNumberIntType, &id);
    printf("%d | %s | %s | %.0f,%.0f %.0fx%.0f | onscreen=%d layer=%d\n", id, owner, name, r.origin.x, r.origin.y, r.size.width, r.size.height, on == kCFBooleanTrue, layer);
  }
  return 0;
}
