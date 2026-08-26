class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.60-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "e7776807f0664cbf0d0728cc236f2415c0981eb8d6557a897d2eeee708641b1d"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "3004474df3cfb529719348f02c9f2f39afa88f0fca469fe8303a9ebe0f3a0034"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.2/dws-linux-arm64.tar.gz"
      sha256 "6386885d10f149c8c555031dda4cf07bf34e1e9daad61d4cd948b92d3c7b7bad"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.2/dws-linux-amd64.tar.gz"
      sha256 "5c94c2af269d2fe5a79a400d4fa3af267a86d6ab21b01a24ede1d29514a6eaef"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.2/dws-skills.zip"
    sha256 "c3bd917f1b44a978ba2a9fbe95c5d0910ccf75f870f1c9b0dc356262ab1080c5"
  end

  def install
    root = Dir["dws-*"].find { |entry| File.directory?(entry) } || "."
    binary = File.join(root, "dws")
    raise "binary not found: #{binary}" unless File.exist?(binary)

    bin.install binary => "dws"

    %w[LICENSE NOTICE README.md CHANGELOG.md].each do |name|
      source = File.join(root, name)
      pkgshare.install source if File.exist?(source)
    end

    skill_dest = pkgshare/"skills/dws"
    skill_dest.mkpath
    resource("skills").stage do
      cp_r(Dir["*"], skill_dest)
    end
  end

  def caveats
    <<~EOS
      Agent Skills are bundled in #{pkgshare}/skills/dws.
      Run `dws skill setup` to install them into your Agent directories.
      This beta is keg-only. Add #{opt_bin} to PATH to use its `dws` binary.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
