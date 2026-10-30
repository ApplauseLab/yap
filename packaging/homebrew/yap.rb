# Source of truth for the Homebrew cask. The update-homebrew-tap job in
# .github/workflows/release.yml rewrites version and sha256 from the published release
# and commits the result to ApplauseLab/homebrew-tap as Casks/yap.rb.
#
# Keeping it here rather than only in the tap means cask changes are reviewed alongside
# the packaging changes they depend on. The placeholder version and sha256 below are
# replaced on release; they are valid values so the file stays parseable and lintable.
cask "yap" do
  version "0.0.0"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"

  url "https://github.com/ApplauseLab/yap/releases/download/v#{version}/Yap-#{version}-macOS.dmg",
      verified: "github.com/ApplauseLab/yap/"
  name "Yap"
  desc "Local-first speech-to-text dictation with a global hotkey"
  homepage "https://github.com/ApplauseLab/yap/"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on arch: :arm64
  # The macOS job builds darwin/arm64 only, and every bundled binary targets macOS 11,
  # the oldest release that runs on Apple Silicon.
  #
  # Homebrew/OSDependsOn wants the bare `macos: :big_sur` symbol here because Big Sur is
  # below Homebrew's own supported floor, but that form means "exactly Big Sur" rather
  # than "or newer". `brew style --fix` rewrites it to `depends_on :macos`, dropping the
  # minimum altogether, and the string form `":big_sur"` passes style but fails to load
  # with "unknown or unsupported macOS version". The comparison string is the only form
  # that both loads and states the real constraint, so the cop is accepted as noise.
  depends_on macos: ">= :big_sur"

  app "Yap.app"

  uninstall quit: "ai.applauselab.yap"

  # Whisper models are downloaded on first run and are gigabytes, so zap matters here.
  # os.UserConfigDir()/yap in internal/models/config.go resolves to this path.
  zap trash: [
    "~/Library/Application Support/yap",
    "~/Library/Saved Application State/ai.applauselab.yap.savedState",
  ]
end
