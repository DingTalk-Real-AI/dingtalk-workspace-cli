class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.51"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.51/dws-darwin-arm64.tar.gz"
      sha256 "025bbb440b9abc099402e8c679a18ff279296aa4d530e43721731f570da0f63d"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.51/dws-darwin-amd64.tar.gz"
      sha256 "5d4a07db95f87ec749a88b7e4598f204fab96102fb50dd31a41fa70c961bf409"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.51/dws-linux-arm64.tar.gz"
      sha256 "1e1a6e3b08adc009950acaa7b1b0a8a3bd327aff7110d1ba6f632a5e76fdfb62"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.51/dws-linux-amd64.tar.gz"
      sha256 "d7b87fe7b9f7ae48467b776dfc08c72fa5fe6a760ca22484ca14efef5eb3df9a"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.51/dws-skills.zip"
    sha256 "265dad520fd28ec7c0025eca389fb2072b3f96eb8ad4e99e5924c484055c0fe5"
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
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
