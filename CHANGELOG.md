# Changelog

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
