include(ExternalProject)
include(GNUInstallDirs)

ExternalProject_Add(evmc
        PREFIX ${CMAKE_SOURCE_DIR}/deps
        DOWNLOAD_NO_PROGRESS 1
        DOWNLOAD_NAME evmc-4ce1c3b3.tar.gz
	URL https://github.com/FISCO-BCOS/evmc/archive/4ce1c3b35346db162443a6975ac8801910533138.tar.gz
            https://osp-1257653870.cos.ap-guangzhou.myqcloud.com/FISCO-BCOS/FISCO-BCOS/deps/evmc-4ce1c3b3.tar.gz
        URL_HASH SHA256=bc75110885ac5524dcbe8af0d3670add5b414697d8154ec2e885b1d6806f9332
        # GIT_REPOSITORY https://github.com/FISCO-BCOS/evmc.git
        # GIT_TAG e0bd9d5dc68ec3a00fe9a3c5e81c98946449a20d
        CMAKE_ARGS -DCMAKE_INSTALL_PREFIX=<INSTALL_DIR>
                -DHUNTER_USE_CACHE_SERVERS=NO
        BUILD_IN_SOURCE 1
        LOG_CONFIGURE 1
        LOG_BUILD 1
        LOG_INSTALL 1
        BUILD_BYPRODUCTS <INSTALL_DIR>/lib/libevmc-instructions.a <INSTALL_DIR>/lib/libevmc-loader.a
)

ExternalProject_Get_Property(evmc INSTALL_DIR)
set(EVMC_INCLUDE_DIRS ${INSTALL_DIR}/include)
file(MAKE_DIRECTORY ${EVMC_INCLUDE_DIRS})  # Must exist.
add_library(EVMC::Loader STATIC IMPORTED)
set(EVMC_LOADER_LIBRARIES ${INSTALL_DIR}/lib/libevmc-loader.a)
set_property(TARGET EVMC::Loader PROPERTY IMPORTED_LOCATION ${EVMC_LOADER_LIBRARIES})
set_property(TARGET EVMC::Loader PROPERTY INTERFACE_INCLUDE_DIRECTORIES ${EVMC_INCLUDE_DIRS})

add_library(EVMC::Instructions STATIC IMPORTED)
set(EVMC_INSTRUCTIONS_LIBRARIES ${INSTALL_DIR}/lib/libevmc-instructions.a)
set_property(TARGET EVMC::Instructions PROPERTY IMPORTED_LOCATION ${EVMC_INSTRUCTIONS_LIBRARIES})
set_property(TARGET EVMC::Instructions PROPERTY INTERFACE_INCLUDE_DIRECTORIES ${EVMC_INCLUDE_DIRS})

add_library(EVMC INTERFACE IMPORTED)
set_property(TARGET EVMC PROPERTY INTERFACE_LINK_LIBRARIES ${EVMC_INSTRUCTIONS_LIBRARIES} ${EVMC_LOADER_LIBRARIES})
set_property(TARGET EVMC PROPERTY INTERFACE_INCLUDE_DIRECTORIES ${EVMC_INCLUDE_DIRS})
add_dependencies(EVMC evmc)
