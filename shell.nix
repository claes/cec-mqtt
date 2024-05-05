{pkgs ? import <nixpkgs> {}}:
pkgs.mkShell {
  #  packages = [ pkgs.go pkgs.gopls pkgs.go-outline pkgs.gotools pkgs.godef pkgs.delve pkgs.mqttui pkgs.libcec pkgs.libcec_platform pkgs.pkg-config pkgs.glibc.static];
  packages = [pkgs.go pkgs.gopls pkgs.go-outline pkgs.gotools pkgs.godef pkgs.delve pkgs.mqttui pkgs.libcec pkgs.libcec_platform pkgs.pkg-config];
}
