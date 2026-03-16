Websocket client devices are "fake" devices that connect to the riverdeck websocket and inform riverdeck of their layout.
They are meant to allow creating custom devices that can be used with riverdeck without needing to implement a full driver for them. This also means that someone can make their own device driver for hardware that is not yet supported by riverdeck, and use the websocket client device to connect it to riverdeck.

Such as creating a "mobile app" that can be used as a device in riverdeck, or creating a "web app" that can be used as a device in riverdeck.
Or even some other options such as creating a "stream deck plugin" that can be used as a device in riverdeck.