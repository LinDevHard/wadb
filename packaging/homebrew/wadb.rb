class Wadb < Formula
  desc "Pair Android devices over ADB Wi-Fi via a terminal QR code"
  homepage "https://github.com/LinDevHard/wadb"
  url "https://github.com/LinDevHard/wadb.git",
      tag:      "v0.2.0",
      revision: "3625509c99eade36e140b6613a97a0a6dcb80946"
  license "MIT"
  head "https://github.com/LinDevHard/wadb.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "."
  end

  def caveats
    <<~EOS
      wadb requires `adb` on your PATH. Install Android platform tools with:
        brew install --cask android-platform-tools
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/wadb --version")
  end
end
