{
  description = "Terminal TUI for AI token usage and cost tracking";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = ["x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin"];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

    releaseVersion = nixpkgs.lib.removeSuffix "\n" (builtins.readFile ./VERSION);
    binVersion = "v${releaseVersion}";
  in {
    packages = forAllSystems (pkgs: rec {
      token-counter = pkgs.buildGoModule {
        pname = "token-counter";
        version = releaseVersion;
        src = self;
        vendorHash = "sha256-J8weZ5B3Jz+VCLhXKPfzhxGTj7B+VwgUAOFBCvYKSeE=";
        ldflags = ["-s" "-w" "-X main.version=${binVersion}"];
        meta = with pkgs.lib; {
          description = "Terminal TUI for AI token usage and cost tracking";
          homepage = "https://github.com/themakunga/token-counter";
          license = licenses.mit;
          platforms = platforms.unix;
          mainProgram = "token-counter";
        };
      };
      default = token-counter;
    });
  };
}
