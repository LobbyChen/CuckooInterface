# Build fix

The original batch file failed when the CMake generator argument was split as:

`-G Visual"` + `Studio 17 2022 -A x64`

This version runs CMake from the package directory and passes the generator as one quoted argument:

`cmake -S . -B build -G "Visual Studio 17 2022" -A x64`

The package also carries the Cuckoo ABI headers under `include/`, so the standalone LuaKernel tree can compile without assuming it lives inside the full CuckooInterface source tree.


Build fixes 3: removed struct/class forward declaration conflict, removed unnecessary current_loading_ state, fixed const callback pointer conversion, and enabled MSVC /utf-8 while relying on Lua's own LUA_USE_WINDOWS definition.
