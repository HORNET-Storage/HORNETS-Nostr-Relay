import { configureStore } from '@reduxjs/toolkit';
import { errorLoggingMiddleware } from '@app/store/middlewares/errorLogging.middleware';
import rootReducer from '@app/store/slices';
import { readRelayMode, persistRelayMode } from '@app/services/localStorage.service';  // Assuming this file is in the same directory

const initialState = {
  mode: {
    relayMode: readRelayMode(),
    // Load the relayMode state from local storage
    kinds: [],
    mediaTypes: [],
  }
};

export const store = configureStore({
  reducer: rootReducer,
  middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(errorLoggingMiddleware),
  preloadedState: initialState, // Use initialState directly here
  });

// export const store = configureStore({
//   reducer: rootReducer,
//   middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(errorLoggingMiddleware),
// });
store.subscribe(() => {
  const currentRelayMode = store.getState().mode.relayMode;
  persistRelayMode(currentRelayMode); // Persist the relayMode state to local storage on change
  });

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;
