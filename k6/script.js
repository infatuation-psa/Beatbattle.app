import http from 'k6/http';
import { sleep } from 'k6';

export default function() {
  http.get('https://dev.beatbattle.app/');
  sleep(1);
}