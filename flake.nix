{
  description = "Terminal TUI for AI token usage and cost tracking";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = ["x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin"];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
  in {
    packages = forAllSystems (pkgs: rec {
      token-counter = pkgs.buildGoModule {
        pname = "token-counter";
        version = "0.3.1";
        src = self;
        vendorHash = "sha256-J8weZ5B3Jz+VCLhXKPfzhxGTj7B+VwgUAOFBCvYKSeE=";
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
