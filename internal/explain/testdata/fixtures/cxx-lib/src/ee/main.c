#include <stdio.h>

const char *greeting(const char *name);

int main(int argc, char *argv[])
{
    printf("%s\n", greeting("PS2"));
    return 0;
}
