cmake_minimum_required(VERSION 3.20)
project(CuckooLuaKernel LANGUAGES C CXX)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# CuckooInterface 根目录；可通过 -DCUCKOO_ROOT_DIR=... 覆盖。
if(NOT DEFINED CUCKOO_ROOT_DIR)
  get_filename_component(CUCKOO_ROOT_DIR "${CMAKE_CURRENT_LIST_DIR}/../../.." ABSOLUTE)
endif()

# 优先使用用户实际 vendored 的 Lua 源码；没有时在 configure 阶段从官方源码下载。
set(CUCKOO_LUA_VENDOR_DIR "${CMAKE_CURRENT_LIST_DIR}/vendor/lua-5.5.1")
set(CUCKOO_LUA_VERSION "5.5.1")
set(CUCKOO_LUA_SHA256 "1c4b4068d67061f2a2231ad2b5422e77acea1487ea9890f6320af614f4373dce")

if(EXISTS "${CUCKOO_LUA_VENDOR_DIR}/src/lapi.c")
  set(LUA_SRC_DIR "${CUCKOO_LUA_VENDOR_DIR}/src")
else()
  include(FetchContent)
  FetchContent_Declare(
    lua_upstream
    URL "https://www.lua.org/ftp/lua-${CUCKOO_LUA_VERSION}.tar.gz"
    URL_HASH "SHA256=${CUCKOO_LUA_SHA256}"
  )
  FetchContent_MakeAvailable(lua_upstream)
  set(LUA_SRC_DIR "${lua_upstream_SOURCE_DIR}/src")
endif()

file(GLOB LUA_C_SOURCES CONFIGURE_DEPENDS "${LUA_SRC_DIR}/*.c")
list(REMOVE_ITEM LUA_C_SOURCES
  "${LUA_SRC_DIR}/lua.c"
  "${LUA_SRC_DIR}/luac.c"
  "${LUA_SRC_DIR}/onelua.c"
)

add_library(cuckoo_lua_kernel SHARED
  lua_kernel.cpp
  ${LUA_C_SOURCES}
)

# 把 Lua 本体源码直接编进 export.dll，而不是链接外部 lua DLL。
target_compile_definitions(cuckoo_lua_kernel PRIVATE
  _CRT_SECURE_NO_WARNINGS
)

target_include_directories(cuckoo_lua_kernel PRIVATE
  "${LUA_SRC_DIR}"
  "${CMAKE_CURRENT_LIST_DIR}/include"
)

if(EXISTS "${CUCKOO_ROOT_DIR}/plugins")
  target_include_directories(cuckoo_lua_kernel PRIVATE "${CUCKOO_ROOT_DIR}/plugins")
endif()

set_target_properties(cuckoo_lua_kernel PROPERTIES
  OUTPUT_NAME "export"
  PREFIX ""
  RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/Release/binary"
  LIBRARY_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/Release/binary"
  ARCHIVE_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/Release/lib"
)

if(MSVC)
  target_compile_options(cuckoo_lua_kernel PRIVATE /W4 /EHsc /utf-8)
endif()

add_custom_command(TARGET cuckoo_lua_kernel POST_BUILD
  COMMAND ${CMAKE_COMMAND} -E copy
    "${CMAKE_CURRENT_LIST_DIR}/META-INF.json"
    "$<TARGET_FILE_DIR:cuckoo_lua_kernel>/../META-INF.json"
)
