# The Go binary is pre-built by CI before the Buildroot build starts.
# It's placed into the rootfs overlay at board/teslausb/rootfs_overlay/usr/bin/teslausb.
# This package definition exists for documentation and dependency tracking.

TESLAUSB_VERSION = 1.0.0
TESLAUSB_SITE = $(BR2_EXTERNAL_TESLAUSB_NEO_PATH)/board/teslausb/rootfs_overlay
TESLAUSB_SITE_METHOD = local
TESLAUSB_INSTALL_TARGET = YES

define TESLAUSB_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/usr/bin/teslausb $(TARGET_DIR)/usr/bin/teslausb
endef

$(eval $(generic-package))
