# Nagios Network Analyzer test instance for #152-#162 (nna_* resources).
#
# Nagios Network Analyzer is not distributed as a container image by Nagios
# Enterprises (VM/OVA appliance or manual install onto a bare OS only) - this
# wraps Nagios's own installer tarball
# (https://assets.nagios.com/downloads/nagios-network-analyzer/nagiosna-latest.tar.gz)
# non-interactively, the same way test/docker/nagiosxi/Dockerfile wraps XI's.
#
# Unlike XI, NNA has no perpetual unlicensed "free mode" - it only offers a
# 30-day trial or a $4,995 perpetual license (see #176). Confirmed live: a
# fresh install's trial activates entirely locally (no license key,
# activation key, or client_id required) and a second independent fresh
# container gets its own full, unblocked 30-day trial - no phone-home check
# ties trial eligibility to this host/network. So this harness stays viable
# indefinitely without a paid license, the same way test/docker/nagiosxi/
# never needed one.
#
# The installer manages MySQL/InfluxDB/Apache/etc. through systemd, so this
# image needs to actually boot systemd as PID 1 (the well-known "systemd in
# Docker" pattern), not just run a chain of `docker build` steps. See
# nna-install.service - the real install happens on first container boot,
# once systemd reaches multi-user.target, not at build time.
#
# NNA's installer supports both el9 and el10 - el9 is used here to reuse the
# already-proven Rocky Linux base/workarounds from test/docker/nagiosxi/.
FROM rockylinux/rockylinux:9-ubi-init

RUN dnf -y --allowerasing install wget tar which curl sudo dnf-plugins-core \
    # hostname isn't in the ubi-init minimal base. fullinstall's nagiosna()
    # step shells out to `hostname -I` to build APP_URL for the .env file;
    # without it, IP_ADDRESS is silently empty, APP_URL ends up as the bare
    # scheme "http://", and the artisan step then hard-fails with Symfony's
    # "Invalid URI" trying to parse it (confirmed live).
    initscripts-service cronie openssh-server hostname \
    && dnf config-manager --set-enabled crb \
    # NNA's prereqs() only auto-installs EPEL for RedHatEnterpriseServer+el10;
    # a Rocky/CentOS-identified system (see the rocky-release patch below)
    # never gets it, but prereqs' package list includes `supervisor`, which
    # only exists in EPEL, not BaseOS/AppStream/CRB - install failed on this
    # (confirmed live: "No match for argument: supervisor") without this.
    && dnf -y install epel-release \
    && dnf clean all

WORKDIR /opt
# Same lack of an official checksum/signature documented in
# test/docker/nagiosxi/Dockerfile - this hash is self-recorded against the
# release current as of this writing (2026R1.5, released 2026-07-08) to
# catch an unexpected content change on a later `docker build`, not to
# provide third-party provenance. If this check starts failing, Nagios has
# shipped a new "latest" build - review what changed before recomputing the
# hash and bumping this comment.
RUN wget -q https://assets.nagios.com/downloads/nagios-network-analyzer/nagiosna-latest.tar.gz \
    && echo 'c44bc61d11d65998aaa0cf0ed56687f32ef09b536ba864dae43a61b303e00a21  nagiosna-latest.tar.gz' | sha256sum -c - \
    && tar xzf nagiosna-latest.tar.gz \
    && rm nagiosna-latest.tar.gz \
    # Same OS-detection gap XI's installer has (CLAUDE.md-documented
    # workaround in test/docker/nagiosxi/Dockerfile): libinstall.sh's
    # set_os_info() checks for centos-release/centos-stream-release/
    # centos-linux-release RPM names but never rocky-release, so distro
    # stays unset and the installer hard-exits with "Could not determine
    # OS." Treat rocky-release as CentOS-equivalent.
    && sed -i 's/rpm -q centos-release; then/rpm -q centos-release || rpm -q rocky-release; then/' \
        /opt/nagiosna/libinstall.sh \
    # fullinstall's selinux() step runs semanage/restorecon/setsebool
    # unconditionally on any non-Debian/Ubuntu distro, but this container
    # has no live SELinux policy store at all (no /sys/fs/selinux) -
    # confirmed live: "SELinux policy is not managed or store cannot be
    # accessed." The firewall() step a few lines earlier has the identical
    # condition text (distro != Ubuntu/Debian) for an unrelated purpose that
    # already works correctly here, so this is scoped to only the
    # selinux() function block via sed's address range, not a global text
    # replace.
    && sed -i '/^selinux() {/,/^}/{s/if \[ "\$distro" != "Ubuntu" \] \&\& \[ "\$distro" != "Debian" \]; then/if [ "\$distro" != "Ubuntu" ] \&\& [ "\$distro" != "Debian" ] \&\& [ -d \/sys\/fs\/selinux ]; then/;}' \
        /opt/nagiosna/fullinstall

# Same php-fpm ACL quirk documented in test/docker/nagiosxi/Dockerfile:
# RHEL/Rocky's php-fpm RPM ships listen.acl_users in its default pool
# config, which requires POSIX ACL support on the socket's directory - the
# /run tmpfs here doesn't support ACLs, so php-fpm's ACL check errors out
# with ENOTSUP and refuses to start (confirmed live: "failed to read the
# ACL of the socket ... Operation not supported"). Without the ACL grant,
# the socket falls back to root:root ownership, which httpd's apache user
# can't connect to - so also set listen.owner/listen.group explicitly.
RUN mkdir -p /etc/systemd/system/php-fpm.service.d \
    && printf '%s\n' \
        '[Service]' \
        'ExecStartPre=/usr/bin/sed -i -e '"'"'s/^listen\.acl_users/;&/'"'"' -e '"'"'s/^;listen\.owner = nobody/listen.owner = apache/'"'"' -e '"'"'s/^;listen\.group = nobody/listen.group = apache/'"'"' /etc/php-fpm.d/www.conf' \
        > /etc/systemd/system/php-fpm.service.d/override.conf

COPY nna-install.service /etc/systemd/system/nna-install.service
RUN mkdir -p /etc/systemd/system/multi-user.target.wants \
    && ln -sf /etc/systemd/system/nna-install.service \
        /etc/systemd/system/multi-user.target.wants/nna-install.service

EXPOSE 80 443

# rockylinux/rockylinux:9-ubi-init's default CMD is /sbin/init (systemd).
# We rely on that rather than overriding CMD/ENTRYPOINT.
