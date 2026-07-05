cmake_minimum_required(VERSION 3.20)

project(113final003 LANGUAGES CXX)

enable_testing()

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

list(APPEND CMAKE_MODULE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/cmake")

set(SOURCE_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/solution" CACHE PATH "Root of student source code")
get_filename_component(SOURCE_ROOT "${SOURCE_ROOT}" ABSOLUTE)

file(GLOB CXX_SOURCE_FILES CONFIGURE_DEPENDS
  "${SOURCE_ROOT}/*.cxx"
  "${SOURCE_ROOT}/*.cpp"
  "${SOURCE_ROOT}/*.cc"
  "${SOURCE_ROOT}/*.c"
)

if(NOT CXX_SOURCE_FILES)
  message(FATAL_ERROR "No C/C++ source files found in SOURCE_ROOT=${SOURCE_ROOT}")
endif()

include(AddJudge)
foreach(CASE_NAME IN ITEMS case1 case2 case3 case4 case5)
  AddJudge("${CASE_NAME}")
endforeach()
