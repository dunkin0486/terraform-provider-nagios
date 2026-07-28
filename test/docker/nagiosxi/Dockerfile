# Nagios XI test instance for provider acceptance tests.
#
# Nagios XI is not distributed as a container image by Nagios Enterprises,
# and there is no OpenAPI spec for its REST API - this wraps Nagios's own
# installer tarball (https://assets.nagios.com/downloads/nagiosxi/xi-latest.tar.gz)
# non-interactively, running in unlicensed "free mode" (7 hosts, unlimited
# services, no license key needed).
#
# The installer manages MySQL/Apache/etc. through systemd, so this image
# needs to actually boot systemd as PID 1 (the well-known "systemd in
# Docker" pattern), not just run a chain of `docker build` steps. See
# nagiosxi-install.service - the real install happens on first container
# boot, once systemd reaches multi-user.target, not at build time.
#
# As of this writing the installer only supports el9 (RHEL/CentOS/Oracle 9),
# Ubuntu 22.04/24.04, and Debian 11/12 - CentOS 6/7 (used by older, now-stale
# community Nagios XI Dockerfiles) are explicitly rejected by the installer's
# check_os_support. Rocky Linux 9 identifies itself as el9-compatible.
FROM rockylinux/rockylinux:10-ubi-init

RUN dnf -y --allowerasing install wget tar which curl sudo dnf-plugins-core \
    # initscripts-service provides `service`, used by the installer's
    # init-mysql to start MySQL - stripped from the ubi-init base image.
    # cronie provides /etc/cron.d, needed by the mrtg subcomponent's
    # post-install script - also stripped from the minimal base image.
    # openssh-server provides sshd.service, which 15-chkconfigalldaemons
    # unconditionally enables alongside the other daemons.
    initscripts-service cronie openssh-server \
    # EPEL packages this installer's Nagios XI dependency RPM needs (e.g.
    # perl-Params-Validate, python3-rrdtool) live in CRB, which Rocky ships
    # disabled by default.
    && dnf config-manager --set-enabled crb \
    && dnf clean all

WORKDIR /opt
RUN wget -q https://assets.nagios.com/downloads/nagiosxi/xi-latest.tar.gz \
    && tar xzf xi-latest.tar.gz \
    && rm xi-latest.tar.gz \
    # The installer's OS detection (get-os-info) only recognizes CentOS/RHEL/Oracle
    # release RPM names - Rocky Linux isn't in its rpm -q checks, so it fails OS
    # detection entirely before ever reaching the (already-supported) el9 code path.
    # Rocky is ABI-compatible with the same el9 codepath CentOS Stream 9 uses, so
    # treat rocky-release as equivalent.
    && sed -i 's/rpm -q centos-release; then/rpm -q centos-release || rpm -q rocky-release; then/' \
        /opt/nagiosxi/get-os-info

# RHEL/Rocky's php-fpm RPM ships listen.acl_users in its default pool config,
# which requires POSIX ACL support on the socket's directory. The /run tmpfs
# in this container doesn't support ACLs (remounting with the acl option
# fails outright here), so php-fpm's ACL check errors out with ENOTSUP and
# refuses to start. Without the ACL grant, the socket falls back to root:root
# ownership (php-fpm's master process binds it before dropping to the `user`/
# `group` directive), which httpd's apache user then can't connect to - so
# also set listen.owner/listen.group explicitly as the ACL-free equivalent.
# This runs via a drop-in whenever php-fpm starts, which is after 01-prereqs
# installs the php-fpm RPM (and its www.conf) mid-install, well before
# 15-chkconfigalldaemons first starts the service.
RUN mkdir -p /etc/systemd/system/php-fpm.service.d \
    && printf '%s\n' \
        '[Service]' \
        'ExecStartPre=/usr/bin/sed -i -e '"'"'s/^listen\.acl_users/;&/'"'"' -e '"'"'s/^;listen\.owner = nobody/listen.owner = apache/'"'"' -e '"'"'s/^;listen\.group = nobody/listen.group = apache/'"'"' /etc/php-fpm.d/www.conf' \
        > /etc/systemd/system/php-fpm.service.d/override.conf

COPY nagiosxi-install.service /etc/systemd/system/nagiosxi-install.service
RUN mkdir -p /etc/systemd/system/multi-user.target.wants \
    && ln -sf /etc/systemd/system/nagiosxi-install.service \
        /etc/systemd/system/multi-user.target.wants/nagiosxi-install.service

EXPOSE 80 443 5666 5667

# rockylinux/rockylinux:9-init's default CMD is /sbin/init (systemd).
# We rely on that rather than overriding CMD/ENTRYPOINT.
