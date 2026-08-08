# Changelog

## [0.1.0-beta.2](https://github.com/RonsenbergVI/fraise/compare/v0.1.0-beta.1...v0.1.0-beta.2) (2026-08-08)


### Features

* add python SDK ([#84](https://github.com/RonsenbergVI/fraise/issues/84)) ([3509036](https://github.com/RonsenbergVI/fraise/commit/35090360620d28321cbc004cd58ce1fe0536d148))


### Maintenance

* **deps:** bump actions/checkout from 4.2.2 to 7.0.1 ([#130](https://github.com/RonsenbergVI/fraise/issues/130)) ([ee1dee0](https://github.com/RonsenbergVI/fraise/commit/ee1dee037ed1700ca1d5bb592b4946e6f28c9108))
* **deps:** bump actions/upload-artifact from 4.6.1 to 7.0.1 ([#125](https://github.com/RonsenbergVI/fraise/issues/125)) ([9e650c2](https://github.com/RonsenbergVI/fraise/commit/9e650c210dbac6ea1791f0357a7d109a7f7b99c7))
* **deps:** bump alpine from 3.20 to 3.24 ([#120](https://github.com/RonsenbergVI/fraise/issues/120)) ([2f95d14](https://github.com/RonsenbergVI/fraise/commit/2f95d148e8d992e1eb6fe31a803cbff4bd6a0a24))
* **deps:** bump github/codeql-action/upload-sarif from 3.37.6 to 4.37.6 ([#129](https://github.com/RonsenbergVI/fraise/issues/129)) ([913226d](https://github.com/RonsenbergVI/fraise/commit/913226dcdcc7f95a6adf48c20ab985c941598219))
* **deps:** bump ossf/scorecard-action from 2.4.1 to 2.4.4 ([#123](https://github.com/RonsenbergVI/fraise/issues/123)) ([1a3e58d](https://github.com/RonsenbergVI/fraise/commit/1a3e58d0d5982f8a88f779b3550270bb9511eafb))
* repository hygiene ([#119](https://github.com/RonsenbergVI/fraise/issues/119)) ([06aed0c](https://github.com/RonsenbergVI/fraise/commit/06aed0c5af69b42e16bd738e029f825002f7c828))

## [0.1.0-beta.1](https://github.com/RonsenbergVI/fraise/compare/v0.1.0-alpha.3...v0.1.0-beta.1) (2026-08-07)


### Bug fixes

* apply recency decay to recall scores ([#90](https://github.com/RonsenbergVI/fraise/issues/90)) ([9ee1077](https://github.com/RonsenbergVI/fraise/commit/9ee1077d630bd80d1ff4c0439f0e9d9a27584941))
* bound scheduler enqueue wait and shed load when the queue stays full ([#92](https://github.com/RonsenbergVI/fraise/issues/92)) ([08f1669](https://github.com/RonsenbergVI/fraise/commit/08f16690b9411915584b02b699b9421cec6cc695))
* make LRU Resize shrink evict immediately ([#88](https://github.com/RonsenbergVI/fraise/issues/88)) ([c3584cf](https://github.com/RonsenbergVI/fraise/commit/c3584cf253497c7384ff0cedbf0a38cc62119338))
* reject out-of-range graph selectors before narrowing ([#87](https://github.com/RonsenbergVI/fraise/issues/87)) ([f2f2f31](https://github.com/RonsenbergVI/fraise/commit/f2f2f3132ccb7499eca4a7cd12c56576dc73a649))
* report the real version in non-release builds and gate it at release ([#113](https://github.com/RonsenbergVI/fraise/issues/113)) ([4091567](https://github.com/RonsenbergVI/fraise/commit/4091567b2f5334a0f859da07896e805477d71c1e))
* surface clause parse errors unmangled to clients ([#91](https://github.com/RonsenbergVI/fraise/issues/91)) ([400914b](https://github.com/RonsenbergVI/fraise/commit/400914b6636d8597e46b729d273be67a20cfef93))


### Performance

* commit writes in place instead of copying the graph ([#89](https://github.com/RonsenbergVI/fraise/issues/89)) ([df742b0](https://github.com/RonsenbergVI/fraise/commit/df742b01586d73c4d19474c4c57166c498122cc4))


### Maintenance

* adopt release-please for versioning and releases ([#114](https://github.com/RonsenbergVI/fraise/issues/114)) ([ef58767](https://github.com/RonsenbergVI/fraise/commit/ef58767b4a51375c7aa63533ef769c1b48375934))
* release 0.1.0-beta.1 and document the squash-merge pitfall ([#116](https://github.com/RonsenbergVI/fraise/issues/116)) ([09ddb6d](https://github.com/RonsenbergVI/fraise/commit/09ddb6de63433894f794c11ca228c8f3e51e704a))
