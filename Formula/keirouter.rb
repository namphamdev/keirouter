# Auto-updated by release.yml on tag v0.1.17. Do not edit manually.
class Keirouter < Formula
  desc "AI API router — unified gateway for 20+ LLM providers with fallback, caching, and dashboard"
  homepage "https://github.com/mydisha/keirouter"
  version "0.1.17"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.17/keirouter_v0.1.17_darwin_arm64.tar.gz"
      sha256 "bcc36705ccadd5a0ec2facab0561d3c3a3059dbec9ba6c66f9d3d35bb9b001e1"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.17/keirouter_v0.1.17_darwin_amd64.tar.gz"
      sha256 "f71b6cf3626a11257ce13b9f398a46ab50113d6455efbd5b6a399453a34eb0b7"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.17/keirouter_v0.1.17_linux_arm64.tar.gz"
      sha256 "b709aeb29850200d6ef0f39666cc22336f1c296ab58c698182281f4b12d31533"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.17/keirouter_v0.1.17_linux_amd64.tar.gz"
      sha256 "461b3b268c98bc362b9cfd086eee041fa96eb1b12b7cc66664ffe8478b10d777"
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
