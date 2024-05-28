import { combineReducers } from '@reduxjs/toolkit';
import userReducer from '@app/store/slices/userSlice';
import authReducer from '@app/store/slices/authSlice';
import nightModeReducer from '@app/store/slices/nightModeSlice';
import themeReducer from '@app/store/slices/themeSlice';
import pwaReducer from '@app/store/slices/pwaSlice';
import serverModeReducer from '@app/store/slices/modeSlice';

// Combine all slice reducers into a single root reducer
const rootReducer = combineReducers({
  user: userReducer,
  auth: authReducer,
  nightMode: nightModeReducer,
  theme: themeReducer,
  pwa: pwaReducer,
  mode: serverModeReducer, // Make sure this name matches what you use in your selectors
});

export default rootReducer;

