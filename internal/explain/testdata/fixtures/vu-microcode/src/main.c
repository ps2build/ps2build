#include <stdio.h>

extern unsigned char vu1_probe_start[];
extern unsigned char vu1_probe_end[];

int main(void) {
	printf("vu1_probe microprogram: %ld bytes\n", (long)(vu1_probe_end - vu1_probe_start));
	return 0;
}
