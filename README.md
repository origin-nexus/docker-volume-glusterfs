# Docker volume plugin for GlusterFS

This managed plugin allows you to mount existing glusterfs volumes (or a subdir) in your containers.


## Features:

- Set servers and volume name at plugin level.
- Gluster logs redirected to docker plugin logs.
- Mutualization of gluster mounts of same volume.

## Usage

### Installation

        docker plugin install --alias <pluginAlias> originnexus/glusterfs-plugin SERVERS=... VOLUME_NAME=... OPTIONS="..." LOGLEVEL=...
    
Accepted variables are:

- **`SERVERS`**: comma seperated list of gluster servers. If set, `servers` will not be configurable during volume creation.
- **`VOLUME_NAME`**: Glusterfs volume name to use. If set, `volume-name` will not be configurable during volume creation.
- **`OPTIONS`**: string of options (space separated), most options from [mount.glusterfs] are accepted, and also `dedicated-mount` (see below). ex: `log-level=ERROR dedicated-mount`
- **`LOGLEVEL`**: log level of the plugin. This will also be the default level for Gluster logs if not set via `log-level` option. Defaults to `WARNING`.
    
### Volume creation
    docker volume create --driver <pluginAlias>  -o <option>=<value> my-volume
    
Accepted options are most options from [mount.glusterfs] and also:

- `servers=...`: comma separated list of gluster servers. If `SERVERS` was set at plugin level, this option is not allowed.
- `volume-name=...`: Glusterfs volume name to use. If `VOLUME_NAME` was set at plugin level, this option is not allowed. The volume must exists on gluster servers, the plugin will not create it.
- `dedicated-mount`: the driver will reuse an existing mount (same `servers` and `volume-name`) unless this option is set. This allows to use the same Gluster volume with different mount options.

If `volume-name` is not set, the plugin will use the name of the docker volume. If set, the plugin will mount a subdir of that gluster volume, creating that subdir if it does not exist.

#### Example:

Assuming *`docker-volumes`* is a gluster replicated volume:

    docker plugin install --alias replicated originnexus/glusterfs-plugin SERVERS=my-gluster-server VOLUME_NAME=docker-volumes
    docker volume create --driver replicated my-volume

All containers using *`my-volume`* will only have access to *`my-volume`* subdirectory in *`docker-volumes`* gluster volume. If that directory did not exists, it would have been created.

This can also be used in compose file:

    volumes:
        my-volume1:
            driver: replicated
        my-volume2:
            driver: replicated

In this case, a container using only *`my-volume1`* will not have access to *`my-volume2`* although they are on the same Gluster volume.

#### Example:
    docker plugin install --alias glusterfs originnexus/glusterfs-plugin
    docker volume create --driver replicated -o servers=my-server my-volume

In this case, the containers will have access to the whole *`my-volume`* Gluster volume.

Compose file would look like:

    volumes:
        my-volume:
            driver: glusterfs
            driver_opts:
                servers: my-server

## Limitations

- Following [mount.glusterfs] options are not supported: `log-file`, `backup-volfile-server` and `backup-volfile-servers`.
- No checks are made on options passed to [mount.glusterfs], and failure will only occur when container mounts the volume.
- Mutualization of gluster mounts in plugin is done on `servers` and `volume-name`, that means that same server names in a different order will trigger a second mount in the plugin.
- No legacy plugin support.

[mount.glusterfs]: http://manpages.ubuntu.com/manpages/focal/man8/mount.glusterfs.8.html