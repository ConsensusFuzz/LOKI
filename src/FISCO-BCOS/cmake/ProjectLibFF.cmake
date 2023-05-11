# Aleth: Ethereum C++ client, tools and libraries.
# Copyright 2017-2019 Aleth Authors.
# Licensed under the GNU General Public License, Version 3.
include(ProjectMPIR)

set(prefix "${CMAKE_BINARY_DIR}/deps")
set(libff_library "${prefix}/lib/${CMAKE_STATIC_LIBRARY_PREFIX}ff${CMAKE_STATIC_LIBRARY_SUFFIX}")
set(libff_inlcude_dir "${prefix}/include/libff")

ExternalProject_Add(libff
    PREFIX "${prefix}"
    DOWNLOAD_NAME libff-03b719a7.tar.gz
    DOWNLOAD_NO_PROGRESS TRUE
    URL https://github.com/scipr-lab/libff/archive/03b719a7c81757071f99fc60be1f7f7694e51390.tar.gz
        https://osp-1257653870.cos.ap-guangzhou.myqcloud.com/FISCO-BCOS/FISCO-BCOS/deps/libff-03b719a7.tar.gz
    URL_HASH SHA256=81b476089af43025c8f253cb1a9b5038a1c375baccffea402fa82042e608ab02
    CMAKE_ARGS
        -DCMAKE_BUILD_TYPE=Release
        -DCMAKE_INSTALL_PREFIX=<INSTALL_DIR>
        -DGMP_INCLUDE_DIR=${MPIR_INCLUDE_DIR}
        -DGMP_LIBRARY=${MPIR_LIBRARY}
        -DCURVE=ALT_BN128
        -DPERFORMANCE=OFF
        -DWITH_PROCPS=OFF
        -DUSE_PT_COMPRESSION=OFF
        -DCMAKE_C_COMPILER=${CMAKE_C_COMPILER}
        -DCMAKE_CXX_COMPILER=${CMAKE_CXX_COMPILER}
        -DCMAKE_CXX_FLAGS=${CMAKE_CXX_FLAGS}
    BUILD_COMMAND ${CMAKE_COMMAND} --build <BINARY_DIR> --config Release
    LOG_CONFIGURE 1
    LOG_DOWNLOAD 1
    LOG_UPDATE 1
    LOG_BUILD 1
    LOG_INSTALL 1
    INSTALL_COMMAND ${CMAKE_COMMAND} --build <BINARY_DIR> --config Release --target install
    BUILD_BYPRODUCTS "${libff_library}"
)
add_dependencies(libff mpir)

# Create snark imported library
add_library(libff::ff STATIC IMPORTED)
file(MAKE_DIRECTORY ${libff_inlcude_dir})
set_property(TARGET libff::ff PROPERTY IMPORTED_CONFIGURATIONS Release)
set_property(TARGET libff::ff PROPERTY IMPORTED_LOCATION_RELEASE ${libff_library})
set_property(TARGET libff::ff PROPERTY INTERFACE_INCLUDE_DIRECTORIES ${libff_inlcude_dir})
set_property(TARGET libff::ff PROPERTY INTERFACE_LINK_LIBRARIES MPIR::mpir)
add_dependencies(libff::ff libff)
