{
  description = "token-counter — terminal TUI for AI token usage and cost";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAll (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          version = self.shortRev or "dev";
        in
        {
          default = pkgs.buildGoModule {
            pname = "token-counter";
            inherit version;
            src = ./.;

            # Run `nix build` once — it will fail and print the correct hash.
            # Replace this value with what it prints.
            vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

            ldflags = [ "-s" "-w" "-X main.version=${version}" ];

            meta = with nixpkgs.lib; {
              description = "Terminal TUI for AI token usage and cost monitoring";
              homepage = "https://github.com/themakunga/token-counter";
              license = licenses.mit;
              mainProgram = "token-counter";
              platforms = platforms.unix;
            };
          };
        });

      apps = forAll (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/token-counter";
        };
      });

      devShells = forAll (system:
        let pkgs = nixpkgs.legacyPackages.${system}; in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [ go pre-commit ];
          };
        });
    };
}
