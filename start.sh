echo $1
if [ "$1" = "bo" ]
then
    echo ${GIT_USER}
    echo ${GIT_PASS}
    docker build -t vp-cap/handler-service .
elif [ "$1" = "br" ]
then
    docker build -t vp-cap/handler-service .
    docker stop handler-service && docker rm handler-service
    docker run --network=common --name  -p 50054:50054 vp-cap/handler-service
else
    docker stop handler-service && docker rm handler-service
    docker run --network=common  --name handler-service -p 50054:50054 vp-cap/handler-service
fi
