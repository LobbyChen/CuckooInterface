# CMakeLists.txt —— MockKernel 构建脚本
#
# 产出：export.dll（CuckooInterface 内核插件入口约定名）
# 用法：
#   cmake -B build -S .
#   cmake --build build --config Release
# 产物位于 build/Release/export.dll
#
# 将 export.dll 与 META-INF.json 打包为 zip：
#   MockKernel.zip
#     ├── META-INF.json
#     └── binary/
#         └── export.dll
# 然后放入 user/Kernel/ 目录即可被 CuckooInterface 加载。

cmake_minimum_required(VERSION 3.15)
project(MockKernel CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# 统一 include 根：指向工程的 plugins/ 目录，源码中使用扁平 include
# （#include "cuckoo_kernel.h"），遵循“唯一搜索根 ↔ 唯一写法”契约。
set(PLUGINS_INCLUDE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../../plugins")

add_library(export SHARED
  mock_kernel.cpp
)

target_include_directories(export PRIVATE
  "${PLUGINS_INCLUDE_DIR}"
  "${CMAKE_CURRENT_SOURCE_DIR}"
)

# 导出符号：使用 __declspec(dllexport)，无需 .def 文件
set_target_properties(export PROPERTIES
  OUTPUT_NAME "export"
  PREFIX ""
)

# Windows 下不生成 lib 前缀，输出名直接为 export.dll
if(WIN32)
  set_target_properties(export PROPERTIES SUFFIX ".dll")
endif()
