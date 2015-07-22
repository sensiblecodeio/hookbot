hookbot
-------

Webhooks. But with outbound connections.

Status: beta. Things may change. We may need your feedback to make it work for you.

What is a web hook?
-------------------

A web hook is simply a HTTP post request made to your server when something happens.
For example, github
[can generate webhooks in response to various actions](https://developer.github.com/webhooks/),
such as when a branch is pushed to.

This is useful for creating a continuous deployment infrastructure, which runs tests
or deploys software when it is updated.

The problem with webhooks is that it requires that the receiver of the hook listens on
a port accessible to the publisher, and that the publisher must be configured to publish
to every listener.

Why use hookbot?
----------------

Hookbot makes it so that you can listen on a public port for webhooks in one place.

Any applications wanting to listen to the webhooks can make an inbound connection.

How do I use hookbot?
---------------------

Hookbot URLs
------------

Advanced features: routers
--------------------------

Hookbot
