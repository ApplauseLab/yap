# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.1](https://github.com/ApplauseLab/yap/compare/v0.2.0...v0.2.1) (2026-05-11)


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
