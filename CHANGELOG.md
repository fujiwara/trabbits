# Changelog

## [v0.0.5](https://github.com/fujiwara/trabbits/compare/v0.0.4...v0.0.5) - 2025-09-10
- Add reload config API endpoint by @fujiwara in https://github.com/fujiwara/trabbits/pull/39
- Add SIGHUP signal handling for configuration reload by @fujiwara in https://github.com/fujiwara/trabbits/pull/41

## [v0.0.4](https://github.com/fujiwara/trabbits/compare/v0.0.3...v0.0.4) - 2025-09-09
- Add Jsonnet configuration file support by @fujiwara in https://github.com/fujiwara/trabbits/pull/36
- Implement server-side diff API endpoint and refactor config handling by @fujiwara in https://github.com/fujiwara/trabbits/pull/38

## [v0.0.3](https://github.com/fujiwara/trabbits/compare/v0.0.2...v0.0.3) - 2025-08-27
- Cluster upstream by @fujiwara in https://github.com/fujiwara/trabbits/pull/32
- Add health check feature for cluster nodes by @fujiwara in https://github.com/fujiwara/trabbits/pull/34
- Refactor configuration to use address format instead of host/port by @fujiwara in https://github.com/fujiwara/trabbits/pull/35

## [v0.0.2](https://github.com/fujiwara/trabbits/compare/v0.0.1...v0.0.2) - 2025-08-20
- SO_REUSEPORT is for linux only by @fujiwara in https://github.com/fujiwara/trabbits/pull/30

## [v0.0.1](https://github.com/fujiwara/trabbits/commits/v0.0.1) - 2025-08-13
- Two upstreams by @fujiwara in https://github.com/fujiwara/trabbits/pull/2
- fix config by @fujiwara in https://github.com/fujiwara/trabbits/pull/3
- auto queue naming. by @fujiwara in https://github.com/fujiwara/trabbits/pull/4
- optimize performance by @fujiwara in https://github.com/fujiwara/trabbits/pull/5
- add API server for monitoring by prometheus exporter. by @fujiwara in https://github.com/fujiwara/trabbits/pull/6
- add queue_attributes to upstream config. by @fujiwara in https://github.com/fujiwara/trabbits/pull/7
- Delete auto generated queue when a consumer is gone. by @fujiwara in https://github.com/fujiwara/trabbits/pull/8
- fix emulates cleanup auto generated queue. by @fujiwara in https://github.com/fujiwara/trabbits/pull/9
- fix tmp queue deletion. by @fujiwara in https://github.com/fujiwara/trabbits/pull/10
- config api by @fujiwara in https://github.com/fujiwara/trabbits/pull/11
- trabbits manage config CLI by @fujiwara in https://github.com/fujiwara/trabbits/pull/12
- supports AMQPLAIN auth, update logging by @fujiwara in https://github.com/fujiwara/trabbits/pull/13
- Bump github.com/alecthomas/kong from 1.9.0 to 1.10.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/16
- Bump github.com/prometheus/common from 0.62.0 to 0.63.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/15
- Supports SO_REUSEPORT by @fujiwara in https://github.com/fujiwara/trabbits/pull/14
- configuration API using unix sock by @fujiwara in https://github.com/fujiwara/trabbits/pull/17
- update readme by @fujiwara in https://github.com/fujiwara/trabbits/pull/18
- Bump golang.org/x/sys from 0.31.0 to 0.32.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/19
- Bump github.com/prometheus/client_golang from 1.21.1 to 1.22.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/20
- Add logger stats metrics by @fujiwara in https://github.com/fujiwara/trabbits/pull/21
- fix metric names. by @fujiwara in https://github.com/fujiwara/trabbits/pull/25
- Bump github.com/alecthomas/kong from 1.10.0 to 1.12.1 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/26
- Bump github.com/prometheus/common from 0.63.0 to 0.65.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/28
- Bump github.com/prometheus/client_golang from 1.22.0 to 1.23.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/27
- Bump golang.org/x/sys from 0.32.0 to 0.34.0 by @dependabot[bot] in https://github.com/fujiwara/trabbits/pull/29
