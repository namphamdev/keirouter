# Auto-updated by release.yml on tag v0.1.15. Do not edit manually.
class Keirouter < Formula
  desc "AI API router — unified gateway for 20+ LLM providers with fallback, caching, and dashboard"
  homepage "https://github.com/mydisha/keirouter"
  version "0.1.15"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.15/keirouter_v0.1.15_darwin_arm64.tar.gz"
      sha256 "49a20b6a852c594ddce2c32b8a3100d5faf459ea8c266c59568171f245256fad"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.15/keirouter_v0.1.15_darwin_amd64.tar.gz"
      sha256 "454989fc220c839e01f01db02415c6be680cc2a02fc6953901148af6294a7ade"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.15/keirouter_v0.1.15_linux_arm64.tar.gz"
      sha256 "003a685041e6b92a3f980772af3ccbf1afed6b466399be7b664754abeff1e5ff"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.15/keirouter_v0.1.15_linux_amd64.tar.gz"
      sha256 "12f71aef8ec5f24fef534c4eda1c0c1ecf16ac359552cd99bc80e825636d22fb"
    end
  end

  def install
    bin.install "keirouter"
    (share/"keirouter").install "frontend"
  end

  def caveats
    <<~EOS
      Quick start:
        keirouter -bootstrap    # create your first API key
        keirouter               # start server on :20180

      Dashboard: http://localhost:20180  (default password: keirouter)
    EOS
  end

  test do
    assert_match "KeiRouter", shell_output("\#{bin}/keirouter --help 2>&1", 2)
  end
end
