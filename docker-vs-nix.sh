docker build -t ticker-printer-basic -f Dockerfile.basic .
docker build -t ticker-printer-distroless -f Dockerfile.distroless .
docker build -t ticker-printer-withbuilder -f Dockerfile.withbuilder .

nix build .#docker
docker load < ./result

docker images
