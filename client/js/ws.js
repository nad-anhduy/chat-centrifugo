export const WS_BASE = 'ws://localhost:8000/connection/websocket';

let centrifuge = null;
let currentSubscriptions = {};
let callbacks = {
    onMessage: null,
    onPresence: null, // "presence_update"
    onSystemEvent: null, // "friend_request_received", "conversation_created"
    onConnect: null,
    onDisconnect: null
};

export const ws = {
    
    setCallbacks: (cb) => {
        callbacks = { ...callbacks, ...cb };
    },

    connect: (token) => {
        if (centrifuge) {
            centrifuge.disconnect();
        }

        centrifuge = new window.Centrifuge(WS_BASE, { token });

        centrifuge.on('connecting', function (ctx) {
            console.log('Centrifugo connecting...');
        }).on('connected', function (ctx) {
            console.log(`Centrifugo connected via ${ctx.transport}`);
            if (callbacks.onConnect) callbacks.onConnect();
        }).on('disconnected', function (ctx) {
            console.log(`Centrifugo disconnected: ${ctx.reason}`);
            if (callbacks.onDisconnect) callbacks.onDisconnect();
        }).connect();
    },

    subscribe: (conversationId) => {
        const channel = `chat:${conversationId}`;
        
        // Don't subscribe twice
        if (currentSubscriptions[channel]) return currentSubscriptions[channel];

        console.log("Subscribing to", channel);
        const sub = centrifuge.newSubscription(channel);
        
        sub.on('publication', function (ctx) {
            const data = ctx.data;
            if (data.Type === 'presence_update') {
                if (callbacks.onPresence) callbacks.onPresence(data);
            } else {
                if (callbacks.onMessage) callbacks.onMessage(data);
            }
        });

        // Handle join/leave for real-time presence
        sub.on('join', function(ctx) {
            console.log("User joined channel:", ctx.info.user);
            if (callbacks.onPresence) {
                callbacks.onPresence({ user_id: ctx.info.user, status: 'ONLINE', type: 'presence_update' });
            }
        });

        sub.on('leave', function(ctx) {
            console.log("User left channel:", ctx.info.user);
            if (callbacks.onPresence) {
                callbacks.onPresence({ user_id: ctx.info.user, status: 'OFFLINE', type: 'presence_update' });
            }
        });

        sub.subscribe();
        currentSubscriptions[channel] = sub;
        return sub;
    },

    subscribeUserChannel: (userId) => {
        const channel = `user:#${userId}`;
        
        if (currentSubscriptions[channel]) return;

        console.log("Subscribing to user namespace", channel);
        const sub = centrifuge.newSubscription(channel);

        sub.on('publication', function (ctx) {
            const data = ctx.data;
            if (callbacks.onSystemEvent) callbacks.onSystemEvent(data);
        });

        sub.subscribe();
        currentSubscriptions[channel] = sub;
    },

    disconnect: () => {
        if (centrifuge) centrifuge.disconnect();
        currentSubscriptions = {};
    }
};
