# Thin sshd overlay for the Code_Aster solver image.
#
# The full docker/codeaster/Dockerfile now bakes the kubeflow mpi-operator
# sshd recipe into its runtime stage and stays root, but a from-source rebuild
# takes ~90 min. This overlay re-applies EXACTLY that change on top of the
# already-published base image so the fix ships in ~1 min:
#
#   docker build -f docker/codeaster/overlay.Dockerfile \
#       -t ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest .
#
# Keep this in sync with the sshd block in docker/codeaster/Dockerfile.
#
# The published base ends as `USER aster`; switch back to root because the
# mpi-operator worker runs `/usr/sbin/sshd -De` (needs root) and mounts SSH keys
# to /root/.ssh. mpirun already runs with --allow-run-as-root.
FROM ghcr.io/theweirdfulmurk/cfd-platform-codeaster:latest

USER root

# sshd is already installed in the base via the `ssh` package — only the host
# keys, the privilege-separation dir, and client/server config are missing.
# `/run/sshd` must exist or `sshd -De` aborts with "Missing privilege
# separation directory" (Ubuntu creates it at boot, not in a container image).
RUN mkdir -p /run/sshd \
    && ssh-keygen -A \
    && printf 'Host *\n    StrictHostKeyChecking no\n    UserKnownHostsFile /dev/null\n    LogLevel ERROR\n' >> /etc/ssh/ssh_config \
    && sed -i '1i StrictModes no' /etc/ssh/sshd_config

WORKDIR /work/aster
ENTRYPOINT ["/bin/bash"]
