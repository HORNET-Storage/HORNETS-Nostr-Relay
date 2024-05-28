
import { PayloadAction, createSlice } from '@reduxjs/toolkit';

// Define the initial state using only serializable values
const initialState = {
  isPWASupported: false,
  isStandalone: window.matchMedia('(display-mode: standalone)').matches,
};

export const pwaSlice = createSlice({
  name: 'pwa',
  initialState,
  reducers: {
    // Action to set the support flag without storing the event
    setPWASupported: (state, action: PayloadAction<boolean>) => {
      state.isPWASupported = action.payload;
    },
    // Optional: Action to clear the flag if needed
    clearPWASupport: (state) => {
      state.isPWASupported = false;
    },
  },
});

// Exporting actions
export const { setPWASupported, clearPWASupport } = pwaSlice.actions;

export default pwaSlice.reducer;


// import { createSlice } from '@reduxjs/toolkit';

// const initialState = {
//   event: null,
//   isPWASupported: false,
//   isStandalone: window.matchMedia('(display-mode: standalone)').matches,
// };

// export const pwaSlice = createSlice({
//   name: 'pwa',
//   initialState,
//   reducers: {
//     addDeferredPrompt: (state, action) => {
//       state.event = action.payload;
//       state.isPWASupported = true;
//     },
//   },
// });

// export const { addDeferredPrompt } = pwaSlice.actions;

// export default pwaSlice.reducer;
