class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.61-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "db9e0d82c5cd8d41bb3c8745b1d3a071b284b7bc4d112ce42451f21cdfba25fc"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "ee2090990e7ee76a257b19917c73b8ff52e3a79a2ee88cbae3c0f13514c26af8"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.1/dws-linux-arm64.tar.gz"
      sha256 "c69e936370f568d35d42cadd678992017a5cb78c574394a49e79383cfd3c4714"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.1/dws-linux-amd64.tar.gz"
      sha256 "3c1a6d9b786548edbc82c5b497250d3b6f3eddb8903ab7b28f436a75846e4079"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.1/dws-skills.zip"
    sha256 "e26b8f91e4c7e94ec993883fe6ef56d4548f985b9571e7f3e9830f3310e605ea"
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
