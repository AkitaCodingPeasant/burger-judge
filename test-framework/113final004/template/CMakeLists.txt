cmake_minimum_required(VERSION 3.20)
project(113final004_submission LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

file(GLOB SUBMISSION_SOURCES CONFIGURE_DEPENDS "*.c" "*.cc" "*.cpp" "*.cxx")
if(NOT SUBMISSION_SOURCES)
  message(FATAL_ERROR "No C/C++ source files found")
endif()

add_executable(${PROJECT_NAME} ${SUBMISSION_SOURCES})
target_include_directories(${PROJECT_NAME} PRIVATE
  "${CMAKE_CURRENT_SOURCE_DIR}"
  "${CMAKE_CURRENT_SOURCE_DIR}/include"
  "${CMAKE_CURRENT_SOURCE_DIR}/runtime"
)
