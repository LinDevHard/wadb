class Wadb < Formula
  desc "Pair Android devices over ADB Wi-Fi using a terminal QR code"
  homepage "https://github.com/LinDevHard/wadb"
  version "1.0.0"
  license "MIT"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/LinDevHard/wadb/releases/download/v1.0.0/wadb-1.0.0-darwin-arm64.tar.gz"
      sha256 "4076bbd6fa0e04e362035db903ac460d29e7ff07046025cca301e3003f836186"
    end

    on_intel do
      url "https://github.com/LinDevHard/wadb/releases/download/v1.0.0/wadb-1.0.0-darwin-amd64.tar.gz"
      sha256 "49efc19d68705270cab0a56fb56097aedc973b6dfd8fc4497faefefd18bd3573"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/LinDevHard/wadb/releases/download/v1.0.0/wadb-1.0.0-linux-arm64.tar.gz"
      sha256 "743f298cd458164d6e7dfca2e97cd04b946134cbb4d7737ce51fa070d79bbcef"
    end

    on_intel do
      url "https://github.com/LinDevHard/wadb/releases/download/v1.0.0/wadb-1.0.0-linux-amd64.tar.gz"
      sha256 "4b9387c11f2c37a9868d4cd13dd7564391d5856480644625803838af1865f744"
    end
  end

  def install
    bin.install "wadb"
    doc.install "README.md"
  end

  def caveats
    <<~EOS
      wadb shells out to adb for Android wireless pairing.
      Install Android platform-tools if adb is not already available:
        brew install --cask android-platform-tools
    EOS
  end

  test do
    assert_match "v#{version}", shell_output("#{bin}/wadb --version")
    assert_match "pair Android devices", shell_output("#{bin}/wadb --help 2>&1")
  end
end
