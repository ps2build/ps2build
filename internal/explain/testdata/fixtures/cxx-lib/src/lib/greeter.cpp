#include <string>

extern "C" const char *greeting(const char *name)
{
    static std::string result;
    result = "Hello, " + std::string(name) + "!";
    return result.c_str();
}
