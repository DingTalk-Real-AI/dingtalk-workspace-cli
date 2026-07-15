class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.52"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52/dws-darwin-arm64.tar.gz"
      sha256 "4f6b4d064a76bcefac42feb5f356253fe43f9499b8cec9d2cdf202e7d3b9b60c"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52/dws-darwin-amd64.tar.gz"
      sha256 "abc87128f4b98d0a01ea99235449031971db8fa4ce94167403e3b736c4b81e9a"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52/dws-linux-arm64.tar.gz"
      sha256 "0d357ef0535f99f2f63b5ecbfdee9c32448be2a2c24f3096c03126b3b7570bc5"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52/dws-linux-amd64.tar.gz"
      sha256 "b7dfd9a4b3489211359261747ed0cb9c8c261434bb762ad3f76df33bdbabd5cb"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52/dws-skills.zip"
    sha256 "0fa3c8dec500c1659e6480d6772ae901b2d12d24322dd5d7283f016024290c21"
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
