class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.64"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.64/dws-darwin-arm64.tar.gz"
      sha256 "6a33ac7573343ba1cfb26ef126cfa1bdd17e126ddfdff49e450bae336107da23"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.64/dws-darwin-amd64.tar.gz"
      sha256 "c3c7cde614012ef217debaeb80b42857b0f49e2227bfd8d88718c7aff8718e26"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.64/dws-linux-arm64.tar.gz"
      sha256 "351504a475b60c51a7b4abeb1a79f4e8de8b2ce1f06f97e0a5791a996bfe29a9"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.64/dws-linux-amd64.tar.gz"
      sha256 "68fd68effdb05c4f7af5771dfc7bf6c10d97d219b3fb992dc99165b198fddb32"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.64/dws-skills.zip"
    sha256 "106999fc60c713aabbcf67fc183553d62feae210bf650a148ed38670468389de"
  end

  def install
    root = Dir["dws-*"].find { |entry| File.directory?(entry) } || "."
    binary = File.join(root, "dws")
    raise "binary not found: #{binary}" unless File.exist?(binary)

    libexec.install binary => "dws"
    bin.install_symlink libexec/"dws"

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
