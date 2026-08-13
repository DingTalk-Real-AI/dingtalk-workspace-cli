class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.58-beta.6"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.6/dws-darwin-arm64.tar.gz"
      sha256 "8f55497b84113f81b318e087c723016a02eded1cb5784cbd0811fe527d5852ca"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.6/dws-darwin-amd64.tar.gz"
      sha256 "b1762f1640310fb4100634fe54d9baace46384bb270c59148fe306275554d491"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.6/dws-linux-arm64.tar.gz"
      sha256 "55393310ef0e1f24ea2c0dc22f00c0eb3b343edc2deb18d0db7838cda65af25d"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.6/dws-linux-amd64.tar.gz"
      sha256 "3830f77d09b4da4aa39f0c08d772ff3fa1ffb837eb15bb417f97edbccb073b7d"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.6/dws-skills.zip"
    sha256 "f304a883a4f9e938b26a44692cd5a8d3d8704ba70ee7f33ba7c288434da72b6f"
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
