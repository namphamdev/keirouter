# Auto-updated by release.yml on tag v0.1.13. Do not edit manually.
class Keirouter < Formula
  desc "AI API router — unified gateway for 20+ LLM providers with fallback, caching, and dashboard"
  homepage "https://github.com/mydisha/keirouter"
  version "0.1.13"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.13/keirouter_v0.1.13_darwin_arm64.tar.gz"
      sha256 "67b6c4f65c6db27a9ee0034f8663b763c2f4c1d50053b52a4b56e0edeb8600db"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.13/keirouter_v0.1.13_darwin_amd64.tar.gz"
      sha256 "256311a0f9618dd7ea62626b94a76bc38230cdba48964f0b8ec85ba3de09dc1d"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.13/keirouter_v0.1.13_linux_arm64.tar.gz"
      sha256 "29891b02fc95b9f10b155d2012685d9e63c2f20bd521574b08dacd3a3fbcdc35"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.13/keirouter_v0.1.13_linux_amd64.tar.gz"
      sha256 "d15638d924abafebfbfe41fe36d343ddf8c8c37d82b99da8c69022ffde7213db"
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
