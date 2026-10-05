# Homebrew formula (tap リポジトリ mikuta0407/homebrew-secon の Formula/secon.rb に置く想定)。
# リリース時に url / sha256 を更新する。
class Secon < Formula
  desc "SoftEther VPN compatible client with virtual NIC and SOCKS5 modes"
  homepage "https://github.com/mikuta0407/secon"
  url "https://github.com/mikuta0407/secon/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  head "https://github.com/mikuta0407/secon.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/secon"
    # GUI は cgo (Cocoa / OpenGL) が必要。Linux では X11 等の開発ライブラリが要るので macOS のみ
    if OS.mac?
      system "go", "build", *std_go_args(output: bin/"secon-gui"), "./cmd/secon-gui"
    end
  end

  def caveats
    <<~EOS
      常駐デーモンを登録するには (仮想 NIC モードには root が必要):
        sudo #{opt_bin}/secon service install
      設定ファイル /etc/secon/config.toml を編集したら:
        secon reload
      SOCKS モードだけならユーザ権限でも動かせます:
        #{opt_bin}/secon service install --user
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/secon version")
  end
end
