function(AddJudge CASE_NAME)
  set(TARGET_NAME "${PROJECT_NAME}-${CASE_NAME}")
  set(CASE_SOURCE "${CMAKE_CURRENT_SOURCE_DIR}/spec/${CASE_NAME}.h")
  set(PRIVATE_INCLUDE_DIR "${CMAKE_CURRENT_BINARY_DIR}/_${CASE_NAME}_")
  set(CASE_DESTINATION "${PRIVATE_INCLUDE_DIR}/case.h")

  if(NOT EXISTS "${CASE_SOURCE}")
    message(FATAL_ERROR "Missing judge case: ${CASE_SOURCE}")
  endif()

  file(SHA3_256 "${CASE_SOURCE}" SECRET)
  file(MAKE_DIRECTORY "${PRIVATE_INCLUDE_DIR}")
  configure_file("${CASE_SOURCE}" "${CASE_DESTINATION}" @ONLY)

  add_executable("${TARGET_NAME}" ${CXX_SOURCE_FILES})
  target_include_directories("${TARGET_NAME}" BEFORE PRIVATE "${PRIVATE_INCLUDE_DIR}")
  target_include_directories("${TARGET_NAME}" PRIVATE
    "${SOURCE_ROOT}"
    "${SOURCE_ROOT}/include"
    "${SOURCE_ROOT}/runtime"
  )

  add_test(NAME "${TARGET_NAME}" COMMAND "${TARGET_NAME}")
endfunction()
