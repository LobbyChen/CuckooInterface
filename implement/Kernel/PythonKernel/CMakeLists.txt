# CMakeLists.txt —— PythonKernel 构建脚本
#
# 产出：export.dll（CuckooInterface 内核插件入口约定名）
# 用法：
#   cmake -B build -S . -A x64
#   cmake --build build --config Release
# 产物位于 build/Release/export.dll
#
# 将 export.dll、sdk/ 目录与 META-INF.json 打包为 zip：
#   PythonKernel.zip
#     ├── META-INF.json
#     ├── binary/
#     │   └── export.dll
#     └── sdk/
#         ├── install.bat
#         └── cuckoo_sdk/
#             ├── __init__.py
#             └── sdk.md
# 然后放入 user/Kernel/ 目录即可被 CuckooInterface 加载。

cmake_minimum_required(VERSION 3.15)
project(PythonKernel CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# 统一 include 根：指向工程的 plugins/ 目录
set(PLUGINS_INCLUDE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../../plugins")

# 查找 Python 开发库
find_package(Python3 COMPONENTS Development REQUIRED)

message(STATUS "Python found: ${Python3_VERSION}")
message(STATUS "Python include: ${Python3_INCLUDE_DIRS}")
message(STATUS "Python libs: ${Python3_LIBRARIES}")

add_library(export SHARED
  python_kernel.cpp
)

target_include_directories(export PRIVATE
  "${PLUGINS_INCLUDE_DIR}"
  "${CMAKE_CURRENT_SOURCE_DIR}"
  "${Python3_INCLUDE_DIRS}"
)

target_link_libraries(export PRIVATE
  "${Python3_LIBRARIES}"
)

# 导出符号：使用 __declspec(dllexport)
set_target_properties(export PROPERTIES
  OUTPUT_NAME "export"
  PREFIX ""
)

if(WIN32)
  set_target_properties(export PROPERTIES SUFFIX ".dll")
endif()
