{
  description = "ytplay — TUI YouTube search & play (Bubble Tea)";

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
      pkg = pkgs:
        pkgs.buildGoModule {
          pname = "ytplay";
          version = "0.1.0";

          src = ./.;
          vendorHash = "sha256-nqRZcIxrL12PSQwn/Ld8Gm5B1CIh4E9vMHNlA1Q8XcY=";

          nativeBuildInputs = [ pkgs.makeWrapper ];

          postInstall = ''
            # Only yt-dlp is provided by the wrapper. mpv is deliberately left to
            # the user's PATH so a custom mpv (e.g. mpv-with-scripts / ModernZ,
            # or a system mpv.conf with osc=no plus a scripted OSC) is not
            # shadowed by a plain nixpkgs mpv.
            wrapProgram $out/bin/ytplay \
              --prefix PATH : ${nixpkgs.lib.makeBinPath [ pkgs.yt-dlp ]}
          '';

          meta = {
            description = "TUI YouTube search & play (Bubble Tea)";
            homepage = "https://github.com/Ssnibles/ytplay";
            license = nixpkgs.lib.licenses.mit;
            mainProgram = "ytplay";
          };
        };
    in
    {
      packages = forAllSystems (system: {
        default = pkg nixpkgs.legacyPackages.${system};
      });
    };
}
