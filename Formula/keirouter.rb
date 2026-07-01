# Auto-updated by release.yml on tag v0.1.22. Do not edit manually.
class Keirouter < Formula
  desc "AI API router — unified gateway for 20+ LLM providers with fallback, caching, and dashboard"
  homepage "https://github.com/mydisha/keirouter"
  version "0.1.22"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.22/keirouter_v0.1.22_darwin_arm64.tar.gz"
      sha256 "a6efb21b8af086d91118cd37ea9661730c12410c519399bf0f63e485c05d76a8"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.22/keirouter_v0.1.22_darwin_amd64.tar.gz"
      sha256 "a7e6a8a1ca9c6efeaf6f77153184b6ea716d093eb86d5479aa77b498c52b40ce"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.22/keirouter_v0.1.22_linux_arm64.tar.gz"
      sha256 "9d62547770166cb02a289d059b90510f3f3a44915a4f8d2c55e2dc58a9be9837"
    else
      url "https://github.com/mydisha/keirouter/releases/download/v0.1.22/keirouter_v0.1.22_linux_amd64.tar.gz"
      sha256 "0ad864cc0830345e6a9d68a4b1e48b9a844114b4ce59cd44019060a18ace8a4d"
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
        keirouter start         # start server on :20180

      Dashboard: http://localhost:20180  (default password: keirouter)
    EOS
  end

  test do
    assert_match "KeiRouter", shell_output("\#{bin}/keirouter --help 2>&1")
  end
end
