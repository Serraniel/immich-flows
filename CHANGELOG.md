# Changelog

## 0.1.0 (2026-10-17)


### Features

* **api:** edit flow files ([#76](https://github.com/Serraniel/immich-flows/issues/76)) ([93e213c](https://github.com/Serraniel/immich-flows/commit/93e213c40db8dacd83ccc821cb3c7fbc11bc61e6)), closes [#28](https://github.com/Serraniel/immich-flows/issues/28)
* authenticate /_flows API with Immich credentials ([#54](https://github.com/Serraniel/immich-flows/issues/54)) ([2b68e5e](https://github.com/Serraniel/immich-flows/commit/2b68e5e654d9572f68ab83bd3764a52b325edcfc))
* **bridge:** asset.uploaded from Immich workflow webhooks ([#84](https://github.com/Serraniel/immich-flows/issues/84)) ([8d0908c](https://github.com/Serraniel/immich-flows/commit/8d0908c753f19b6ac679848236fd017bcf7ff77f)), closes [#31](https://github.com/Serraniel/immich-flows/issues/31)
* **engine:** executor and run log ([#56](https://github.com/Serraniel/immich-flows/issues/56)) ([fdd9c0b](https://github.com/Serraniel/immich-flows/commit/fdd9c0b75854acc163a73ad999e824b7287cbd38)), closes [#15](https://github.com/Serraniel/immich-flows/issues/15)
* **engine:** expressions and templates ([#43](https://github.com/Serraniel/immich-flows/issues/43)) ([05b8816](https://github.com/Serraniel/immich-flows/commit/05b8816ce6b24af623cec20a16a159b49bede4f0)), closes [#14](https://github.com/Serraniel/immich-flows/issues/14)
* **engine:** flow YAML schema ([#42](https://github.com/Serraniel/immich-flows/issues/42)) ([d86eb4f](https://github.com/Serraniel/immich-flows/commit/d86eb4f99df65b7aa6adb24eb75eb6503ef5dd9d)), closes [#13](https://github.com/Serraniel/immich-flows/issues/13)
* **events:** asset trashed, deleted, restored ([#69](https://github.com/Serraniel/immich-flows/issues/69)) ([0b93f1b](https://github.com/Serraniel/immich-flows/commit/0b93f1b22fc8e22b3dba25637f4ec61f9f831103)), closes [#10](https://github.com/Serraniel/immich-flows/issues/10)
* **events:** detect deletes made by Immich itself ([#82](https://github.com/Serraniel/immich-flows/issues/82)) ([0f07ec3](https://github.com/Serraniel/immich-flows/commit/0f07ec3aec8656c18c23008c787d0263adc1c35d)), closes [#32](https://github.com/Serraniel/immich-flows/issues/32)
* **events:** duplicates resolved ([#70](https://github.com/Serraniel/immich-flows/issues/70)) ([a8d7a98](https://github.com/Serraniel/immich-flows/commit/a8d7a98a9ab4c06fdc07fed8b2b7d6c2f30aa8cb)), closes [#11](https://github.com/Serraniel/immich-flows/issues/11)
* **events:** persistent job queue with retries ([#55](https://github.com/Serraniel/immich-flows/issues/55)) ([3c28218](https://github.com/Serraniel/immich-flows/commit/3c28218107e57c628371f3124096a22d5d718aa4)), closes [#9](https://github.com/Serraniel/immich-flows/issues/9)
* **events:** tag assigned and removed ([#71](https://github.com/Serraniel/immich-flows/issues/71)) ([4b040b9](https://github.com/Serraniel/immich-flows/commit/4b040b9eeac7bb70c74411c999899f2c16a339b8)), closes [#12](https://github.com/Serraniel/immich-flows/issues/12)
* **flows:** load flows from the flows directory ([#63](https://github.com/Serraniel/immich-flows/issues/63)) ([80eca3e](https://github.com/Serraniel/immich-flows/commit/80eca3e572001685ff6ad727e15e9d77f8de515e)), closes [#21](https://github.com/Serraniel/immich-flows/issues/21)
* generated Immich API client ([#39](https://github.com/Serraniel/immich-flows/issues/39)) ([701cc5d](https://github.com/Serraniel/immich-flows/commit/701cc5d821fac4a77990ace17631311d64ccde06)), closes [#2](https://github.com/Serraniel/immich-flows/issues/2)
* per-user Immich connection with scoped API key ([#57](https://github.com/Serraniel/immich-flows/issues/57)) ([372ebe0](https://github.com/Serraniel/immich-flows/commit/372ebe0dbb39b5dad37198a2b2e52684cd49abc3)), closes [#5](https://github.com/Serraniel/immich-flows/issues/5)
* **proxy:** hash original upload while streaming ([#62](https://github.com/Serraniel/immich-flows/issues/62)) ([3ae0e47](https://github.com/Serraniel/immich-flows/commit/3ae0e47f72437a47e9a9f0f5b583cb524337e797)), closes [#8](https://github.com/Serraniel/immich-flows/issues/8)
* **proxy:** route hook framework ([#41](https://github.com/Serraniel/immich-flows/issues/41)) ([f8d7d01](https://github.com/Serraniel/immich-flows/commit/f8d7d01711e819a3fd4f2be4818a9ea08f7799ed)), closes [#7](https://github.com/Serraniel/immich-flows/issues/7)
* SQLite store with migrations ([#40](https://github.com/Serraniel/immich-flows/issues/40)) ([e5eabc9](https://github.com/Serraniel/immich-flows/commit/e5eabc9ed80fcf833db58c04b3cd524cef905fb0)), closes [#3](https://github.com/Serraniel/immich-flows/issues/3)
* **steps:** hash ([#66](https://github.com/Serraniel/immich-flows/issues/66)) ([52e9319](https://github.com/Serraniel/immich-flows/commit/52e9319e96d9723e78d043818bffed7906462c0f)), closes [#16](https://github.com/Serraniel/immich-flows/issues/16)
* **steps:** http request ([#85](https://github.com/Serraniel/immich-flows/issues/85)) ([663c2da](https://github.com/Serraniel/immich-flows/commit/663c2da7cbb6ae28c83ada4efa123d9c9e3aff80)), closes [#37](https://github.com/Serraniel/immich-flows/issues/37)
* **steps:** metadata.set ([#68](https://github.com/Serraniel/immich-flows/issues/68)) ([7c3bc28](https://github.com/Serraniel/immich-flows/commit/7c3bc2837cd95ad098b4cf3dc9c13515a230d757)), closes [#20](https://github.com/Serraniel/immich-flows/issues/20)
* **steps:** regex, split, set ([#64](https://github.com/Serraniel/immich-flows/issues/64)) ([71d229c](https://github.com/Serraniel/immich-flows/commit/71d229c4639d1c14e6458d0ae5302cb30e6ffc97)), closes [#17](https://github.com/Serraniel/immich-flows/issues/17)
* **steps:** tag.merge ([#67](https://github.com/Serraniel/immich-flows/issues/67)) ([234c849](https://github.com/Serraniel/immich-flows/commit/234c84955cba9532e0005609bde74e23fbddda1b)), closes [#19](https://github.com/Serraniel/immich-flows/issues/19)
* **steps:** tag.read, tag.assign, tag.remove ([#65](https://github.com/Serraniel/immich-flows/issues/65)) ([74e72eb](https://github.com/Serraniel/immich-flows/commit/74e72eb9755820beb64ebd1d41c5c76030b5b309)), closes [#18](https://github.com/Serraniel/immich-flows/issues/18)
* **tags:** bulk operations by pattern ([#73](https://github.com/Serraniel/immich-flows/issues/73)) ([2b926cb](https://github.com/Serraniel/immich-flows/commit/2b926cbe0605e7cc0f1559b3b80d942d660338df)), closes [#26](https://github.com/Serraniel/immich-flows/issues/26)
* **tags:** rename, move and merge API ([#60](https://github.com/Serraniel/immich-flows/issues/60)) ([d30fba0](https://github.com/Serraniel/immich-flows/commit/d30fba07368f6f761304ca12169e015ef3288d6f))
* **trigger:** incoming webhook ([#86](https://github.com/Serraniel/immich-flows/issues/86)) ([8e73fbf](https://github.com/Serraniel/immich-flows/commit/8e73fbf6123dc243de49f5aec4847da7afe8b5d0)), closes [#36](https://github.com/Serraniel/immich-flows/issues/36)
* **trigger:** manual runs and backfill ([#72](https://github.com/Serraniel/immich-flows/issues/72)) ([de31047](https://github.com/Serraniel/immich-flows/commit/de310472c88624e5696f25a8a561d0ce68463159)), closes [#22](https://github.com/Serraniel/immich-flows/issues/22)
* **trigger:** schedule ([#87](https://github.com/Serraniel/immich-flows/issues/87)) ([5a4e7e0](https://github.com/Serraniel/immich-flows/commit/5a4e7e0ef0e103db8efb3b944e7617ea2e36f6e5)), closes [#35](https://github.com/Serraniel/immich-flows/issues/35)
* **ui:** flow list and YAML editor ([#77](https://github.com/Serraniel/immich-flows/issues/77)) ([b93e13e](https://github.com/Serraniel/immich-flows/commit/b93e13e7413b5628cd85f70cade9340363c2b189)), closes [#28](https://github.com/Serraniel/immich-flows/issues/28)
* **ui:** run a flow on all existing assets ([#88](https://github.com/Serraniel/immich-flows/issues/88)) ([11ff398](https://github.com/Serraniel/immich-flows/commit/11ff398f1e0e689b387590da93862167c0c10d92))
* **ui:** run history and logs ([#74](https://github.com/Serraniel/immich-flows/issues/74)) ([5ae778b](https://github.com/Serraniel/immich-flows/commit/5ae778b146302ae4134349e825ce7f2e6c0ccf9f)), closes [#30](https://github.com/Serraniel/immich-flows/issues/30)
* **ui:** SvelteKit scaffold embedded in binary ([#45](https://github.com/Serraniel/immich-flows/issues/45)) ([fea48e3](https://github.com/Serraniel/immich-flows/commit/fea48e3d86f32b5ee29f15c4147cae4137e0c7d9))
* **ui:** tag management page ([#75](https://github.com/Serraniel/immich-flows/issues/75)) ([e60e22e](https://github.com/Serraniel/immich-flows/commit/e60e22e69115174edd53f1af6309c16e77bf1a6a)), closes [#27](https://github.com/Serraniel/immich-flows/issues/27)
* **ui:** visual flow designer ([#81](https://github.com/Serraniel/immich-flows/issues/81)) ([0cc8e26](https://github.com/Serraniel/immich-flows/commit/0cc8e26fe746504bb50c150974776d56fea4a5e0)), closes [#29](https://github.com/Serraniel/immich-flows/issues/29)
* **ui:** warn when the user is not connected ([#92](https://github.com/Serraniel/immich-flows/issues/92)) ([39c7af9](https://github.com/Serraniel/immich-flows/commit/39c7af99c51a20c8b78bbb7d58d421b530f92b50)), closes [#89](https://github.com/Serraniel/immich-flows/issues/89)


### Bug Fixes

* **hooks:** trigger upload flows even if the original cannot be hashed ([#95](https://github.com/Serraniel/immich-flows/issues/95)) ([382e148](https://github.com/Serraniel/immich-flows/commit/382e148dedc19ea8deac866c5345c803da493821))
* **proxy:** read hooked JSON responses with any content type ([#93](https://github.com/Serraniel/immich-flows/issues/93)) ([1f53e7a](https://github.com/Serraniel/immich-flows/commit/1f53e7a6769f9ec66a06e27290ec6e7763ea6ba1))


### Performance

* proxy load and memory test ([#83](https://github.com/Serraniel/immich-flows/issues/83)) ([2c1c7ee](https://github.com/Serraniel/immich-flows/commit/2c1c7ee5b27a63136a9bca5e5f20615ba38c07b8)), closes [#33](https://github.com/Serraniel/immich-flows/issues/33)


### Documentation

* flow reference and cookbook ([#78](https://github.com/Serraniel/immich-flows/issues/78)) ([3a23124](https://github.com/Serraniel/immich-flows/commit/3a231249f348fb7158e5284c421af41aa05b9cbd)), closes [#23](https://github.com/Serraniel/immich-flows/issues/23)
* install, reverse proxy, backup and upgrades ([#80](https://github.com/Serraniel/immich-flows/issues/80)) ([8c82a0d](https://github.com/Serraniel/immich-flows/commit/8c82a0d991692f5810334722b00cd2acd954b2fc)), closes [#34](https://github.com/Serraniel/immich-flows/issues/34)
