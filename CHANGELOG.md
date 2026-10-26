# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0](https://github.com/ApplauseLab/yap/compare/v0.4.2...v0.5.0) (2026-10-26)


### Features

* add onboarding flow with model selection and permission setup ([53ae76f](https://github.com/ApplauseLab/yap/commit/53ae76fdcecdd7e1be27be3698076bdfe5fe9c34))
* customizable hotkeys and cross-platform hotkey support ([#1](https://github.com/ApplauseLab/yap/issues/1)) ([eea51d1](https://github.com/ApplauseLab/yap/commit/eea51d1fe6b2282f331336a7f814a89130e0c632))
* Models Library with safe downloads, redesigned UI, and multilingual fix ([#18](https://github.com/ApplauseLab/yap/issues/18)) ([43c42c2](https://github.com/ApplauseLab/yap/commit/43c42c22378f4545f9229e71ac595fe85b25c4f2)), closes [#5](https://github.com/ApplauseLab/yap/issues/5)
* polished permissions onboarding inspired by Logi Options+ ([e15a187](https://github.com/ApplauseLab/yap/commit/e15a1874419b3e867372242a4cb209d72d76ff37))
* polished permissions onboarding inspired by Logi Options+ ([#4](https://github.com/ApplauseLab/yap/issues/4)) ([e15a187](https://github.com/ApplauseLab/yap/commit/e15a1874419b3e867372242a4cb209d72d76ff37))
* **ui:** tidy the Home Get started list ([#20](https://github.com/ApplauseLab/yap/issues/20)) ([3e6ba69](https://github.com/ApplauseLab/yap/commit/3e6ba69523e545230321d10b9aed44da490bf764))


### Bug Fixes

* add NSIS to PATH and fix Windows build output ([61841ad](https://github.com/ApplauseLab/yap/commit/61841ad23f475d636ab7d57ebf86449095a6768f))
* add portaudio DLL to PATH for Windows build ([52d8080](https://github.com/ApplauseLab/yap/commit/52d80800a7d45c563e5aec1279fb80a09df334ff))
* add RequestAccessibilityPermissions stub for non-darwin platforms ([cd24691](https://github.com/ApplauseLab/yap/commit/cd24691b1baefcc53ff5d23f9a65300784c0238f))
* bundle PortAudio dylib in macOS app for self-contained distribution ([3eef2fd](https://github.com/ApplauseLab/yap/commit/3eef2fd856b9826a61138fc478e20afe3c7f3e91))
* bundle PortAudio in Linux AppImage for self-contained distribution ([9ccb6e1](https://github.com/ApplauseLab/yap/commit/9ccb6e1b664d5054366018c513f50bb7ef73e7ff))
* copy PortAudio DLL before NSIS installer runs ([84e9890](https://github.com/ApplauseLab/yap/commit/84e98906c46ab7426790d52f087553176f2e42fb))
* icon transparency and DMG layout ([1619eda](https://github.com/ApplauseLab/yap/commit/1619eda235246d6e5b90fa0844c11ae0701f48ee))
* include PortAudio DLL in Windows builds and reduce redundant macOS permission prompts ([caa20c5](https://github.com/ApplauseLab/yap/commit/caa20c59945426f38ba1c23ba4d6ad1275d35ecb))
* **macos:** use Metal for local transcription and pin the release toolchain ([#15](https://github.com/ApplauseLab/yap/issues/15)) ([511de9b](https://github.com/ApplauseLab/yap/commit/511de9bfee86d90424fa7da1f92c6e912e883756))
* persistent whisper worker, transcription readiness, and release hardening ([#8](https://github.com/ApplauseLab/yap/issues/8)) ([dc13018](https://github.com/ApplauseLab/yap/commit/dc13018a9d2d26a86af0b3f9464d3f6358e3cd3e))
* preserve crash diagnostics and retain macOS paste target ([#10](https://github.com/ApplauseLab/yap/issues/10)) ([#16](https://github.com/ApplauseLab/yap/issues/16)) ([969033c](https://github.com/ApplauseLab/yap/commit/969033cd0d541dd6724c667d4fa55d2760e63ddd))
* re-sign macOS app after bundling PortAudio to fix 'damaged' error ([0ef6130](https://github.com/ApplauseLab/yap/commit/0ef6130e183886165c84217bb783de1017398252))
* reap sound player children and stop whisper-server when Yap dies ([#13](https://github.com/ApplauseLab/yap/issues/13)) ([0389bb2](https://github.com/ApplauseLab/yap/commit/0389bb2c5a0829a74aebfc483d36cc9097198617))
* resolve CI build failures for all platforms ([620b195](https://github.com/ApplauseLab/yap/commit/620b1950219c388eb3d1e413421253052549de74))
* resolve FUSE dependency issue for Ubuntu 24.04 AppImage build ([afda1a1](https://github.com/ApplauseLab/yap/commit/afda1a1ba0e33b4a3a5f583a8c19125e444374e7))
* show platform-correct hotkey labels in recording overlays ([#19](https://github.com/ApplauseLab/yap/issues/19)) ([9a736ac](https://github.com/ApplauseLab/yap/commit/9a736ac544ec49efd3653fbc6eb974cabd5e62f4))
* skip Windows build for now, release macOS and Linux only ([c218145](https://github.com/ApplauseLab/yap/commit/c21814573ec60de2cd40c625c568c3272227b6e2))
* **windows:** verify and harden the local transcription runtime ([#14](https://github.com/ApplauseLab/yap/issues/14)) ([2593ac6](https://github.com/ApplauseLab/yap/commit/2593ac60fed0b14913ae179829163a9fd9819515))


### Code Refactoring

* use dynamic version from package.json in settings footer ([c9b2f92](https://github.com/ApplauseLab/yap/commit/c9b2f92c3d7ee4f70cb3e73c90a955012a25ad64))


### Documentation

* add documentation links to README ([b2c505f](https://github.com/ApplauseLab/yap/commit/b2c505fdad52574e660da22eed49df5a60a2c874))
* add polished README and MIT license ([aaeeee2](https://github.com/ApplauseLab/yap/commit/aaeeee207b2a8b9dda3523f0bb45250fdd57c2f1))
* rename to ApplauseWhisper (no space) ([ed7e8e7](https://github.com/ApplauseLab/yap/commit/ed7e8e7e0763b4936447ec56fb8201e2b246b106))
* update README icon to ear design ([09b9a69](https://github.com/ApplauseLab/yap/commit/09b9a692a3c659e747361f3f76e39473ee2005d4))
* use app icon PNG for README instead of SVG ([8f2e775](https://github.com/ApplauseLab/yap/commit/8f2e7757aae612388dc017c39fe8a3900bf28384))
* use SVG icon with rounded background for README ([53de6e7](https://github.com/ApplauseLab/yap/commit/53de6e7cbe09eeed249af7c94410708bf99eef0c))

## [0.4.0](https://github.com/ApplauseLab/yap/compare/v0.3.0...v0.4.0) (2026-06-01)


### Features

* polished permissions onboarding inspired by Logi Options+ ([e15a187](https://github.com/ApplauseLab/yap/commit/e15a1874419b3e867372242a4cb209d72d76ff37))
* polished permissions onboarding inspired by Logi Options+ ([#4](https://github.com/ApplauseLab/yap/issues/4)) ([e15a187](https://github.com/ApplauseLab/yap/commit/e15a1874419b3e867372242a4cb209d72d76ff37))

## [0.3.0](https://github.com/ApplauseLab/yap/compare/v0.2.0...v0.3.0) (2026-05-22)


### Features

* customizable hotkeys and cross-platform hotkey support ([#1](https://github.com/ApplauseLab/yap/issues/1)) ([eea51d1](https://github.com/ApplauseLab/yap/commit/eea51d1fe6b2282f331336a7f814a89130e0c632))


### Bug Fixes

* bundle PortAudio dylib in macOS app for self-contained distribution ([3eef2fd](https://github.com/ApplauseLab/yap/commit/3eef2fd856b9826a61138fc478e20afe3c7f3e91))
* bundle PortAudio in Linux AppImage for self-contained distribution ([9ccb6e1](https://github.com/ApplauseLab/yap/commit/9ccb6e1b664d5054366018c513f50bb7ef73e7ff))
* copy PortAudio DLL before NSIS installer runs ([84e9890](https://github.com/ApplauseLab/yap/commit/84e98906c46ab7426790d52f087553176f2e42fb))
* icon transparency and DMG layout ([1619eda](https://github.com/ApplauseLab/yap/commit/1619eda235246d6e5b90fa0844c11ae0701f48ee))
* include PortAudio DLL in Windows builds and reduce redundant macOS permission prompts ([caa20c5](https://github.com/ApplauseLab/yap/commit/caa20c59945426f38ba1c23ba4d6ad1275d35ecb))
* re-sign macOS app after bundling PortAudio to fix 'damaged' error ([0ef6130](https://github.com/ApplauseLab/yap/commit/0ef6130e183886165c84217bb783de1017398252))

## [0.1.0](https://github.com/ApplauseLab/yap/releases/tag/v0.1.0) (2026-05-07)

### Features

* Initial release of Yap
* Speech-to-text transcription using OpenAI Whisper API or local whisper.cpp
* Cross-platform support for macOS, Windows, and Linux
* Real-time audio recording and transcription
* Usage statistics tracking
