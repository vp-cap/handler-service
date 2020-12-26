echo $1
if [ "$1" = "bo" ]
then
    echo ${GIT_USER}
    echo ${GIT_PASS}
    docker build --build-arg GIT_USER=${GIT_USER} --build-arg GIT_PASS=${GIT_PASS} -t handler-service .
elif [ "$1" = "br" ]
then
    docker build --build-arg GIT_USER=${GIT_USER} --build-arg GIT_PASS=${GIT_PASS} -t handler-service .
    docker stop handler-service && docker rm handler-service
    docker run --name  -p 50052:50052 handler-service
elsehandler-service
    docker stop handler-service && docker rm handler-service
    docker run --name handler-service -p 50052:50052 handler-service
fi
