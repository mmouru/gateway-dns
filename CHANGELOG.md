# Changelog

## [0.3.0](https://github.com/mmouru/gateway-dns/compare/v0.2.0...v0.3.0) (2026-10-31)


### Features

* **tailscale:** require gateways to have hostname for tailscale splitdns feature ([428fe4b](https://github.com/mmouru/gateway-dns/commit/428fe4b420b04641ef75e0b0eb4feca7e434f07f))


### Bug Fixes

* add gofmt to makefile and fix linting ([2e9ee2b](https://github.com/mmouru/gateway-dns/commit/2e9ee2b2587bbe7d03faa6efe11a9f26e7c2981c))
* **tailscale-client:** set the resolvers ip to splitdns instead of gateways ([91b0041](https://github.com/mmouru/gateway-dns/commit/91b00411b26ca9bffc0604bd2295377e5455c6b2))


### Miscellaneous

* add test to makefile ([b53c93e](https://github.com/mmouru/gateway-dns/commit/b53c93ea19e6bac6d3bee4e1d02240369ad6907b))
* **git-hooks:** add precommit and commit msg hooks for this repo ([5af4576](https://github.com/mmouru/gateway-dns/commit/5af4576da497ce1d38b32c172b4e3cc4ac595bab))
* **README:** add info about gateway specs with tailscale ([627652b](https://github.com/mmouru/gateway-dns/commit/627652b660c24b35e9bdc4fd6c6c5ad42fa56403))
* **README:** make tailscale integration collapsible ([8da4736](https://github.com/mmouru/gateway-dns/commit/8da4736725ee534e529ed0928f6a6eac39552da3))
* **tailscale-integ:** rename nameserver client to dns server ([ce3f496](https://github.com/mmouru/gateway-dns/commit/ce3f496c0a2633a2f3898e6fcababa4518a58ccd))
* **tailscale-integ:** rename nameserver client to dns server ([97331ff](https://github.com/mmouru/gateway-dns/commit/97331ff9b04fc9a54395b4d02262ed642fea68a2))
* **ts-client:** move the all inialize code to the controller ([2ea95e5](https://github.com/mmouru/gateway-dns/commit/2ea95e566f7587d8be762cef76026226df02baef))

## [0.2.0](https://github.com/mmouru/gateway-dns/compare/v0.1.2...v0.2.0) (2026-10-19)


### Features

* add optional tailscale split-dns control code ([8f5bf64](https://github.com/mmouru/gateway-dns/commit/8f5bf64d155d8dc8092473f97d7dd8bb81e5733f))
* add tailscale client creation to main ([c2b0897](https://github.com/mmouru/gateway-dns/commit/c2b08979e043ede24cffd60c45176f62569b772c))


### Bug Fixes

* gofmt findings ([b2cc947](https://github.com/mmouru/gateway-dns/commit/b2cc947d58f417a87dbecccd68d86fd36fb5097a))


### Miscellaneous

* add custom name for tags in makefile ([fa18156](https://github.com/mmouru/gateway-dns/commit/fa1815669be028ec996030012d27cb8968fe3cca))
* add tests to tailscale nameserver client ([10b9a7f](https://github.com/mmouru/gateway-dns/commit/10b9a7f656f45933c610a5335502d4f63127aafd))

## [0.1.2](https://github.com/marttimourujarvi/gateway-dns/compare/v0.1.1...v0.1.2) (2026-09-30)


### Bug Fixes

* **imports:** fix the outdated modules and run goimports formatting ([6ce962c](https://github.com/marttimourujarvi/gateway-dns/commit/6ce962c7120edc9325f33cc4ce82527cec4c5ba3))


### Miscellaneous

* **go-version:** bump go version to 1.27.1 ([5415c88](https://github.com/marttimourujarvi/gateway-dns/commit/5415c885abd5c7b77a1826a1029a78cb375282ef))
* **go-version:** bump go version to 1.27.1 ([44a68b1](https://github.com/marttimourujarvi/gateway-dns/commit/44a68b1769865c86fb2b3886cde9deed6cc1b34d))
* **release-process:** update release practice for this repo, docume… ([4f433a9](https://github.com/marttimourujarvi/gateway-dns/commit/4f433a9fe8dc51b007adc2e31fdbdb62a6e506ed))
* **release-process:** update release practice for this repo, document in README ([b995932](https://github.com/marttimourujarvi/gateway-dns/commit/b995932c7afddb6377c96f27b5b1330c8bb2a4b0))
* update one line deploy for gateway-dns ([32d31a9](https://github.com/marttimourujarvi/gateway-dns/commit/32d31a9aeac5e1943ad521785b3170c6bdc17e73))

## [0.1.1](https://github.com/marttimourujarvi/gateway-dns/compare/v0.1.0...v0.1.1) (2026-09-29)


### Bug Fixes

* move the service to same gateway-dns namespace, add name for udp port ([818957d](https://github.com/marttimourujarvi/gateway-dns/commit/818957d26013f4f41a3c5b780205260fd0af4670))
