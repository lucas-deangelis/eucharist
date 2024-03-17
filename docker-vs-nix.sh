docker build -t basic -f Dockerfile.basic .
docker build -t distroless -f Dockerfile.distroless .
docker build -t withbuilder -f Dockerfile.withbuilder .

nix build .#docker
docker load < ./result

docker images
