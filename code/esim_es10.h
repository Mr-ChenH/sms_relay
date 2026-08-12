#ifndef ESIM_ES10_H
#define ESIM_ES10_H

#include <Arduino.h>

bool esimES10Command(const uint8_t* request, size_t requestLength, uint8_t** response, size_t* responseLength);

#endif
