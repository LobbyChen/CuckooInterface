# Copyright (C) 2026 LobbyChen
# SPDX-License-Identifier: AGPL-3.0-only
# CMakeLists.txt —— PythonKernel 构建脚本
#
# 不再链接 Python 导入库：运行时通过 python_runtime.cpp 动态加载
# python3.dll / python3XX.dll（CPython 稳定 ABI），因此：
#   - 构建机不需要安装对应版本的 Python 开发包；
#   - 同一个 export.dll 可以在任意 CPython 3.x 上运行；
#   - 打包时把 embeddable Python 放入 binary/python 即可离线运行。
#
# 用法：
#   cmake -B build -S . -A x64
#   cmake --build build --config Release
# 产物位于 build/Release/export.dll

cmake_minimum_required(VERSION 3.15)
project(PythonKernel CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(POLICY CMP0091)
  cmake_policy(SET CMP0091 NEW)
endif()

# 统一 include 根：指向工程的 plugins/ 目录
set(PLUGINS_INCLUDE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../../plugins")

add_library(export SHARED
  python_kernel.cpp
  python_runtime.cpp
)

target_include_directories(export PRIVATE
  "${PLUGINS_INCLUDE_DIR}"
  "${CMAKE_CURRENT_SOURCE_DIR}"
)

# 导出符号：使用 __declspec(dllexport)
set_target_properties(export PROPERTIES
  OUTPUT_NAME "export"
  PREFIX ""
)

if(WIN32)
  set_target_properties(export PROPERTIES SUFFIX ".dll")
endif()

if(MSVC)
  # 静态链接 CRT，避免目标机器缺少 VCRUNTIME140.dll。
  set_target_properties(export PROPERTIES
    MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>"
  )
  target_compile_options(export PRIVATE /W4 /EHsc /utf-8)
else()
  # MinGW：静态链接 libgcc/libstdc++，Kernel 可单文件分发。
  target_compile_options(export PRIVATE
    -Wall -Wextra -Wno-unused-parameter -Wno-cast-function-type)
  target_link_options(export PRIVATE -static -static-libgcc -static-libstdc++ -s)
endif()
