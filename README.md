# service-profile

The profile service repository contains code necessary to run the service that identifies ant investor actors. This service will answer the question related to who is this and how details like how do we contact them.

### How do I get set up? ###

* The api definition is found at bitbucket.org/antinvestor/api/service
* To update the proto service you need to run the command :
    `protoc -I ../api/service/profile/v1/ ../api/service/profile/v1/papi.proto --go_out=plugins=grpc:grpc/profile`
    `protoc -I ../api/service/health/v1/ ../api/service/health/v1/health.proto --go_out=plugins=grpc:grpc/health`
    `protoc -I ../api/service/notification/v1/ ../api/service/notification/v1/notification.proto --go_out=plugins=grpc:grpc/notification`

    with that in place update the implementation appropriately considering the profile project bare bones.

* Database fixtures and migrations are combined and will be run automatically before the container starts during deployments.

* Dependencies
    Running this project requires an sql database and access to the authentication service
    
* How to run tests execute the command :
    `go test ./...`
    
* Deployment :

    This is an automated process done via bitbucket pipelines.
    Once the code is ready for deployment, merging to develop deploys to the staging environment.
    Merging to master deploys to production so all changes must be thoroughly validated before merging.
    

