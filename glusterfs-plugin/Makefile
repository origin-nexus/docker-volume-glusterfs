PLUGIN_NAME = originnexus/glusterfs-plugin
PLUGIN_TAG ?= dev
BUILD_DIR = _build

all: clean rootfs create

clean:
	@echo "### rm ./${BUILD_DIR}"
	@rm -rf ./${BUILD_DIR}

rootfs:
	@echo "### docker build rootfs image"
	@docker build -f Dockerfile -t ${PLUGIN_NAME}:rootfs ..
	@echo "### create rootfs directory in ./${BUILD_DIR}/rootfs"
	@mkdir -p ./${BUILD_DIR}/rootfs
	@docker create --name gluster-tmp ${PLUGIN_NAME}:rootfs
	@docker export gluster-tmp | tar -x -C ./${BUILD_DIR}/rootfs
	@echo "### copy config.json to ./${BUILD_DIR}/"
	@cp config.json ./${BUILD_DIR}/
	@docker rm -vf gluster-tmp

create:
	@echo "### remove existing plugin ${PLUGIN_NAME}:${PLUGIN_TAG} if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	@echo "### create new plugin ${PLUGIN_NAME}:${PLUGIN_TAG} from ./${BUILD_DIR}"
	@docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} ./${BUILD_DIR}

enable:		
	@echo "### enable plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"		
	@docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}

push:  clean rootfs create enable
	@echo "### push plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}
