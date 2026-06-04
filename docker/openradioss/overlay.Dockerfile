# Thin sshd overlay for the OpenRadioss solver image.
#
# The full docker/openradioss/Dockerfile now bakes the kubeflow mpi-operator
# sshd recipe into its runtime stage, but a from-source rebuild takes ~45 min.
# This overlay re-applies EXACTLY that sshd layer on top of the already-published
# base image so the fix ships in ~1 min:
#
#   docker build -f docker/openradioss/overlay.Dockerfile \
#       -t ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest .
#
# Keep this in sync with the sshd block in docker/openradioss/Dockerfile.
FROM ghcr.io/theweirdfulmurk/cfd-platform-openradioss:latest

USER root

# The published base predates the sshd additions, so install the server/client
# here too. Worker pods run `/usr/sbin/sshd -De`; the launcher ssh-es in.
# Generate host keys; disable client host-key prompt and server StrictModes
# (the operator mounts /root/.ssh world-readable).
RUN dnf install -y openssh-server openssh-clients && dnf clean all \
    && ssh-keygen -A \
    && printf 'Host *\n    StrictHostKeyChecking no\n    UserKnownHostsFile /dev/null\n    LogLevel ERROR\n' >> /etc/ssh/ssh_config \
    && sed -i '1i StrictModes no' /etc/ssh/sshd_config

WORKDIR /work
ENTRYPOINT ["/bin/bash"]
