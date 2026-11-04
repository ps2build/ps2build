#include <kernel.h>
#include <sifrpc.h>
#include <loadfile.h>
#include <stdio.h>

#include "helper_irx.h"

int main(int argc, char *argv[])
{
    sceSifInitRpc(0);

    int id = SifExecModuleBuffer(helper_irx, size_helper_irx, 0, NULL, NULL);
    printf("loaded helper module, id=%d\n", id);

    return 0;
}
