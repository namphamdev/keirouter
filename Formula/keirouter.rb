# Auto-updated by release.yml on tag v0.1.3. Do not edit manually.
class Keirouter < Formula
  desc "AI API router — unified gateway for 20+ LLM providers with fallback, caching, and dashboard"
  homepage "https://github.com/mydisha/keirouter"
  version "0.1.3"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.3/keirouter_v0.1.3_darwin_arm64.tar.gz"
      sha256 "fa8ac427bf2d307ee8cdac789081d7bfd26bcc28343fc186febdef36a1457ab4"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.3/keirouter_v0.1.3_darwin_amd64.tar.gz"
      sha256 "a76d0aede1ef43fd48e7031fc0e53c066f685b7f333029265172ed8027f1447c"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.3/keirouter_v0.1.3_linux_arm64.tar.gz"
      sha256 "b1f6f84e89c3bf3427785a0248e0165b09258d205b22d2ae0a4e1668179cab51"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.3/keirouter_v0.1.3_linux_amd64.tar.gz"
      sha256 "04c6ef29943deb647aafe2093bb743d322814a53e4b0963ff99bfa130993ecc4"
    end
  end

  def install
    bin.install "keirouter"
    share.install "frontend" => "keirouter/frontend"
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
